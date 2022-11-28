// Package actioncable provides a server-side implementation of the Rails Action Cable protocol
// (https://docs.anycable.io/misc/action_cable_protocol)
package actioncable

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/sargassum-world/godest/marshaling"
)

// Subprotocols

const (
	ActionCableV1JSONSubprotocol    = "actioncable-v1-json"
	ActionCableV1MsgpackSubprotocol = "actioncable-v1-msgpack"
)

// Error Handling

// isNormalClose checks whether the error indicates a websocket connection closing under ordinary
// conditions.
func isNormalClose(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
}

// filterNormalClose returns the underlying error when the wrapped error is from a websocket
// connection closing under ordinary conditions.
func filterNormalClose(underlying error, wrapped error) error {
	if isNormalClose(underlying) {
		// Return the raw error so the Serve function can act differently on a normal close
		return underlying
	}
	return wrapped
}

type ErrorSanitizer func(err error) string

// Message Handling

// Handler handles Action Cable channel subscription and channel action messages.
type Handler interface {
	HandleSubscription(ctx context.Context, sub *Subscription) error
	HandleAction(ctx context.Context, identifier, data string) error
}

// Conn

// Conn represents a server-side Action Cable connection.
type Conn struct {
	wsc             *websocket.Conn
	toClient        chan serverMessage
	h               Handler
	unsubscribers   map[string]func()
	sanitizeError   ErrorSanitizer
	subprotocol     string
	marshaledWSType int
	marshaler       marshaling.Marshaler
}

// ConnOption modifies a [Conn]. For use with [Upgrade].
type ConnOption func(conn *Conn)

// WithErrorSanitizer creates a [ConnOption] to set the Conn's error sanitizer, which processes
// errors to avoid leaking sensitive information in server logs.
func WithErrorSanitizer(f ErrorSanitizer) ConnOption {
	return func(conn *Conn) {
		conn.sanitizeError = f
	}
}

// defaultErrorSanitizer replaces errors with a generic message.
func defaultErrorSanitizer(err error) string {
	if err == nil {
		return ""
	}

	if errors.Is(err, context.Canceled) {
		return "logged out"
	}
	// Sanitize the error message to avoid leaking information from Serve method errors
	return "server or client error"
}

// Upgrade upgrades the WebSocket connection to an Action Cable connection.
func Upgrade(wsc *websocket.Conn, handler Handler, opts ...ConnOption) (conn *Conn, err error) {
	subprotocol := wsc.Subprotocol()
	var marshaler marshaling.Marshaler
	var messageType int

	switch subprotocol {
	default:
		return nil, errors.Errorf("unsupported subprotocol %s", subprotocol)
	case ActionCableV1JSONSubprotocol:
		messageType = websocket.TextMessage
		marshaler = marshaling.JSON{}
	case ActionCableV1MsgpackSubprotocol:
		messageType = websocket.BinaryMessage
		marshaler = marshaling.MessagePack{}
	}
	// TODO: check wsc for its subprotocol so we can handle different encodings
	conn = &Conn{
		wsc:             wsc,
		toClient:        make(chan serverMessage),
		h:               handler,
		sanitizeError:   defaultErrorSanitizer,
		unsubscribers:   make(map[string]func()),
		subprotocol:     subprotocol,
		marshaledWSType: messageType,
		marshaler:       marshaler,
	}
	for _, opt := range opts {
		opt(conn)
	}
	return conn, nil
}

// disconnect cancels all subscriptions and sends an Action Cable disconnect message.
func (c *Conn) disconnect(serr error, allowReconnect bool) {
	// Because c.unsubscribers isn't protected by a mutex, the Close method should only be called
	// after the Serve method has completed.
	for _, unsubscriber := range c.unsubscribers {
		unsubscriber()
	}
	c.unsubscribers = make(map[string]func())
	// We leave the toClient channel open because Subscriptions can send into it, and the sendAll
	// method doesn't need to detect whether toClient is closed; Subscriptions can just detect that
	// the connection is done if there's no receiver on the channel.

	// We send close messages only as a courtesy; they may fail if the client already closed the
	// websocket connection by going away, so we don't care about such errors; we need to call the
	// websocket's Close method regardless.
	_ = c.writeAsMarshaled(newDisconnect(c.sanitizeError(serr), allowReconnect))
}

// Close cancels all subscriptions, sends an Action Cable disconnect message, and closes the
// WebSocket connection. The Conn should not be used after being closed.
func (c *Conn) Close(err error) error {
	// We send close messages only as a courtesy; they may fail if the client already closed the
	// websocket connection by going away, so we don't care about such errors; we need to call the
	// websocket's Close method regardless.
	// TODO: is there any situation where we want to allow reconnection?
	c.disconnect(err, false)
	_ = c.writeMessage(websocket.CloseMessage, []byte{})

	return errors.Wrap(c.wsc.Close(), "couldn't close websocket")
}

// Receiving

// wsPongWait is the WebSocket connection read timeout duration.
const wsPongWait = 60 * time.Second

// subscribe processes an Action Cable subscribe command.
func (c *Conn) subscribe(ctx context.Context, identifier string) error {
	if _, ok := c.unsubscribers[identifier]; ok {
		// the subscriber already has a subscription, so just confirm it again.
		c.toClient <- newSubscriptionConfirmation(identifier)
		return nil
	}

	cctx, cancel := context.WithCancel(ctx)
	if err := c.h.HandleSubscription(
		cctx, &Subscription{
			identifier: identifier,
			toClient:   c.toClient,
		},
	); err != nil {
		c.toClient <- newSubscriptionRejection(identifier)
		cancel()
		return errors.Wrap(err, "subscribe command handler encountered error")
	}
	c.unsubscribers[identifier] = cancel
	c.toClient <- newSubscriptionConfirmation(identifier)
	return nil
}

// receive processes an Action Cable command.
func (c *Conn) receive(ctx context.Context, command clientMessage) error {
	switch command.Command {
	default:
		return errors.Errorf("unknown command %s", command.Command)
	case subscribeCommand:
		return c.subscribe(ctx, command.Identifier)
	case unsubscribeCommand:
		unsubscriber, ok := c.unsubscribers[command.Identifier]
		if !ok || unsubscriber == nil {
			return nil
		}
		unsubscriber()
		delete(c.unsubscribers, command.Identifier)
	case actionCommand:
		if err := c.h.HandleAction(ctx, command.Identifier, command.Data); err != nil {
			return errors.Wrap(err, "action command handler encountered error")
		}
	}
	return nil
}

// readAsMarshaled reads a value from a marshaled string or bytes.
func (c *Conn) readAsMarshaled(result any) error {
	messageType, marshaled, err := c.wsc.ReadMessage()
	if err != nil {
		return filterNormalClose(err, errors.Wrap(err, "couldn't read websocket message"))
	}
	if messageType != c.marshaledWSType {
		return errors.Errorf(
			"unexpected websocket message type %d (expected %d)", messageType, c.marshaledWSType,
		)
	}
	if uerr := c.marshaler.Unmarshal(marshaled, result); uerr != nil {
		return errors.Wrap(uerr, "couldn't unmarshal websocket message")
	}
	return nil
}

// receiveAll processes WebSocket pongs and Action Cable commands.
func (c *Conn) receiveAll(ctx context.Context) (err error) {
	if err = c.wsc.SetReadDeadline(time.Now().Add(wsPongWait)); err != nil {
		return errors.Wrap(err, "couldn't set read deadline")
	}
	c.wsc.SetPongHandler(func(string) error {
		return errors.Wrap(
			c.wsc.SetReadDeadline(time.Now().Add(wsPongWait)), "couldn't set read deadline",
		)
	})

	for {
		var command clientMessage
		received := make(chan struct{})
		go func() {
			// ReadJSON blocks for a while due to websocket read timeout, but we don't want it to delay
			// context cancelation so we launch it and synchronize with a closable channel
			err = c.readAsMarshaled(&command) // FIXME: possible data race on err? If so, synchronize!
			close(received)
		}()
		select {
		case <-ctx.Done():
			// We wait for received to be closed to avoid a data race where the goroutine for ReadJSON
			// would set err after this select case has already returned an error. We ignore any error
			// from ReadJSON (e.g. broken pipe resulting from browser tab closure, which also cancels the
			// context) because we don't care about reading data after the context is canceled.
			<-received
			return ctx.Err()
		case <-received:
			if err != nil {
				return filterNormalClose(err, errors.Wrap(err, "couldn't unmarshal client message"))
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = c.receive(ctx, command); err != nil {
				return err
			}
		}
	}
}

// Sending

// resetWriteDeadline pushes back the write deadline by 10 sec.
func (c *Conn) resetWriteDeadline() error {
	const wsWriteWait = 10 * time.Second
	return errors.Wrap(
		c.wsc.SetWriteDeadline(time.Now().Add(wsWriteWait)), "couldn't reset write deadline",
	)
}

// writeMessage sends binary data as a WebSocket message.
func (c *Conn) writeMessage(messageType int, data []byte) error {
	if err := c.resetWriteDeadline(); err != nil {
		return err
	}
	return errors.Wrap(
		c.wsc.WriteMessage(messageType, data), "couldn't write data over websocket connection",
	)
}

// writeAsMarshaled sends a value as a marshaled string or bytes.
func (c *Conn) writeAsMarshaled(v any) error {
	marshaled, err := c.marshaler.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "couldn't marshal data to write over websocket connection")
	}

	return c.writeMessage(c.marshaledWSType, marshaled)
}

// sendAll sends all WebSocket pings, Action Cable handshakes and pings, and Action Cable messages
// from the toClient queue.
func (c *Conn) sendAll(ctx context.Context) (err error) {
	const (
		wsPingFraction  = 9
		wsPingPeriod    = wsPongWait * wsPingFraction / 10
		cablePingPeriod = 3 * time.Second
	)
	wsPingTicker := time.NewTicker(wsPingPeriod)
	defer wsPingTicker.Stop()
	cablePingTicker := time.NewTicker(cablePingPeriod)
	defer cablePingTicker.Stop()

	if err = c.resetWriteDeadline(); err != nil {
		return err
	}
	if err = c.writeAsMarshaled(newWelcome()); err != nil {
		return errors.Wrap(err, "couldn't send welcome message for Action Cable handshake")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wsPingTicker.C:
			if err = ctx.Err(); err != nil {
				// Context was also canceled, it should have priority
				return err
			}
			if err = c.writeMessage(
				websocket.PingMessage, []byte(fmt.Sprintf("%d", time.Now().Unix())),
			); err != nil {
				return filterNormalClose(err, errors.Wrap(err, "couldn't send websocket ping"))
			}
		case <-cablePingTicker.C:
			if err = ctx.Err(); err != nil {
				// Context was also canceled, it should have priority
				return err
			}
			if err = c.writeAsMarshaled(newPing(time.Now())); err != nil {
				return filterNormalClose(err, errors.Wrap(err, "couldn't send Action Cable ping"))
			}
		case message := <-c.toClient:
			if err = ctx.Err(); err != nil {
				// Context was also canceled, it should have priority
				return err
			}
			if err = c.writeAsMarshaled(message); err != nil {
				return filterNormalClose(err, errors.Wrap(
					err, "couldn't send Action Cable server message for client",
				))
			}
		}
	}
}

// Serving

// Serve processes all WebSocket ping/pong messages and Action Cable messages.
func (c *Conn) Serve(ctx context.Context) (err error) {
	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return c.receiveAll(egctx)
	})
	eg.Go(func() error {
		return c.sendAll(egctx)
	})
	if err = eg.Wait(); err != nil {
		if isNormalClose(err) {
			return nil
		}
		return err
	}
	return nil
}
