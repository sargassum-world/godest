package turbostreams

import (
	"bytes"
	"context"
	"io"

	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/pubsub"
)

// Context

type brokerContext = pubsub.BrokerContext[*Context, Message]

// Context represents the context of the current Turbo Streams pub-sub broker event.
type Context struct {
	*brokerContext

	sessionID string
	messages  []Message
	rendered  *bytes.Buffer
}

// SessionID returns the ID of the cookie session associated with the HTTP request which created
// the connection to the client for Turbo Streams. Only valid for SUB, UNSUB, and MSG handlers.
func (c *Context) SessionID() string {
	return c.sessionID
}

// Published returns the messages to be rendered. Only valid for MSG handlers.
func (c *Context) Published() []Message {
	return c.messages
}

// MsgWriter returns an [io.Writer] to write rendered messages to. Only valid for MSG handlers.
func (c *Context) MsgWriter() io.Writer {
	return c.rendered
}

// Handlers

type (
	// HandlerFunc defines a function to handle Turbo Streams pub-sub events. Analogous to Echo's
	// HandlerFunc.
	HandlerFunc = pubsub.HandlerFunc[*Context]
	// MiddlewareFunc defines a function to process middleware. Analogous to Echo's MiddlewareFunc.
	MiddlewareFunc = pubsub.MiddlewareFunc[*Context]
)

// EmptyHandler is a handler which does nothing.
func EmptyHandler(c *Context) error {
	return nil
}

// Broker

type (
	// Hub coordinates broadcasting of messages between Turbo Streams message publishers and
	// subscribers.
	Hub = pubsub.Hub[[]Message]
	// Route contains a handler and information for matching against requests. Analogous to Echo's
	// Route.
	Route = pubsub.Route
)

// Turbo Streams pub-sub broker event methods.
const (
	MethodPub   = pubsub.MethodPub
	MethodSub   = pubsub.MethodSub
	MethodUnsub = pubsub.MethodUnsub
	MethodMsg   = "MSG"
)

// Broker is the pub-sub framework for routing Turbo Streams pub-sub events to handlers. Analogous
// to Echo's Echo.
type Broker struct {
	broker *pubsub.Broker[*Context, Message]
	logger pubsub.Logger
}

// NewBroker creates an instance of [Broker].
func NewBroker(logger pubsub.Logger) *Broker {
	return &Broker{
		broker: pubsub.NewBroker[*Context, Message](logger),
		logger: logger,
	}
}

// Hub returns the associated pub-sub [Hub].
func (b *Broker) Hub() *Hub {
	return b.broker.Hub()
}

// PUB registers a new PUB route for a stream name with matching handler in the router, with
// optional route-level middleware. Refer to [Broker.Serve] for details on how PUB handlers are
// used.
func (b *Broker) PUB(streamName string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(pubsub.MethodPub, streamName, h, m...)
}

// SUB registers a new SUB route for a stream name with matching handler in the router, with
// optional route-level middleware. Refer to [Broker.Subscribe] for details on how SUB handlers are
// used.
func (b *Broker) SUB(streamName string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(pubsub.MethodSub, streamName, h, m...)
}

// UNSUB registers a new UNSUB route for a stream name with matching handler in the router, with
// optional route-level middleware. Refer to [Broker.Subscribe] for details on how UNSUB handlers
// are used.
func (b *Broker) UNSUB(streamName string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(pubsub.MethodUnsub, streamName, h, m...)
}

// MSG registers a new MSG route for a stream name with matching handler in the router, with
// optional route-level middleware. Refer to [Broker.Subscribe] for details on how SUB handlers are
// used.
func (b *Broker) MSG(streamName string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(MethodMsg, streamName, h, m...)
}

// Use adds middleware to the chain which is run after the router. Analogous to Echo's Echo.Use.
func (b *Broker) Use(middleware ...MiddlewareFunc) {
	b.broker.Use(middleware...)
}

// triggerMsg looks up and runs the MSG handler associated with the Turbo Streams stream name.
func (b *Broker) triggerMsg(
	ctx context.Context, streamName, sessionID string, messages []Message,
) (rendered string, err error) {
	c := &Context{
		brokerContext: b.broker.NewBrokerContext(ctx, MethodMsg, streamName),
		sessionID:     sessionID,
		messages:      messages,
		rendered:      &bytes.Buffer{},
	}
	h := b.broker.GetHandler(MethodMsg, streamName, c.RouterContext())
	if err := h(c); err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error(errors.Wrapf(h(c), "couldn't trigger msg on stream %s", streamName))
		return "", err
	}
	return c.rendered.String(), nil
}

// Subscribe runs the SUB handler for the stream name and, if it does not produce an error, adds a
// subscription to the broker's Hub with a callback function to handle messages broadcast over the
// broker's Hub. When the context is canceled, the UNSUB handler is run. Any messages published on
// the broker's Hub (e.g. messages broadcast from [Context.Publish], [Context.Broadcast], or
// [Hub.Broadcast]) will be routed to the MSG handler and exposed via [Context.Published]; the
// handler should render the messages into Turbo Streams HTML messages and write the result to
// [Context.MsgWriter]. The resulting message will then be passed to the message consumer callback
// function.
func (b *Broker) Subscribe(
	ctx context.Context, streamName, sessionID string,
	msgConsumer func(ctx context.Context, rendered string) error,
) (finished <-chan struct{}) {
	return b.broker.Subscribe(
		ctx, streamName,
		func(c *pubsub.BrokerContext[*Context, Message]) *Context {
			return &Context{
				brokerContext: c,
				sessionID:     sessionID,
			}
		},
		func(ctx context.Context, messages []Message) error {
			rendered, err := b.triggerMsg(ctx, streamName, sessionID, messages)
			if err != nil {
				b.logger.Error(errors.Wrapf(err, "msg handler on stream %s failed", streamName))
				return err
			}
			return msgConsumer(ctx, rendered)
		},
	)
}

// Serve launches and cancels PUB handlers based on the appearance and disappearance of
// subscriptions for the PUB handlers' corresponding stream names. The PUB handler for a stream name
// is started in a goroutine when a new subscription is added to the broker (or to the broker's Hub)
// on a stream name which previously did not have associated subscriptions; and its context is
// canceled upon removal of the only remaining subscription for the stream name. This way, exactly
// one instance of the PUB handler for a stream name is run exactly when there is at least one
// subscriber on that stream name. The Broker should not be used after the Serve method finishes
// running.
func (b *Broker) Serve(ctx context.Context) error {
	return b.broker.Serve(ctx, func(c *pubsub.BrokerContext[*Context, Message]) *Context {
		return &Context{
			brokerContext: c,
		}
	})
}

// Router is the subset of [Broker] methods for adding handlers to routes.
type Router interface {
	pubsub.Router[*Context]
	MSG(streamName string, h HandlerFunc, m ...MiddlewareFunc) *Route
}
