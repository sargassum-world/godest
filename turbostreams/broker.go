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

type Context struct {
	*brokerContext

	sessionID string
	messages  []Message
	rendered  *bytes.Buffer
}

func (c *Context) SessionID() string {
	return c.sessionID
}

func (c *Context) Published() []Message {
	return c.messages
}

func (c *Context) MsgWriter() io.Writer {
	return c.rendered
}

// Handlers

type (
	HandlerFunc    = pubsub.HandlerFunc[*Context]
	MiddlewareFunc = pubsub.MiddlewareFunc[*Context]
)

func EmptyHandler(c *Context) error {
	return nil
}

// Broker

type (
	Hub   = pubsub.Hub[[]Message]
	Route = pubsub.Route
)

const (
	MethodPub   = pubsub.MethodPub
	MethodSub   = pubsub.MethodSub
	MethodUnsub = pubsub.MethodUnsub
	MethodMsg   = "MSG"
)

type Broker struct {
	broker *pubsub.Broker[*Context, Message]
	logger pubsub.Logger
}

func NewBroker(logger pubsub.Logger) *Broker {
	return &Broker{
		broker: pubsub.NewBroker[*Context, Message](logger),
		logger: logger,
	}
}

func (b *Broker) Hub() *Hub {
	return b.broker.Hub()
}

func (b *Broker) PUB(topic string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(pubsub.MethodPub, topic, h, m...)
}

func (b *Broker) SUB(topic string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(pubsub.MethodSub, topic, h, m...)
}

func (b *Broker) UNSUB(topic string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(pubsub.MethodUnsub, topic, h, m...)
}

func (b *Broker) MSG(topic string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return b.broker.Add(MethodMsg, topic, h, m...)
}

func (b *Broker) Use(middleware ...MiddlewareFunc) {
	b.broker.Use(middleware...)
}

func (b *Broker) triggerMsg(
	ctx context.Context, topic, sessionID string, messages []Message,
) (rendered string, err error) {
	c := &Context{
		brokerContext: b.broker.NewBrokerContext(ctx, MethodMsg, topic),
		sessionID:     sessionID,
		messages:      messages,
		rendered:      &bytes.Buffer{},
	}
	h := b.broker.GetHandler(MethodMsg, topic, c.RouterContext())
	if err := h(c); err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error(errors.Wrapf(h(c), "couldn't trigger msg on topic %s", topic))
		return "", err
	}
	return c.rendered.String(), nil
}

func (b *Broker) Subscribe(
	ctx context.Context, topic, sessionID string,
	msgConsumer func(ctx context.Context, rendered string) (ok bool),
) (unsubscriber func(), finished <-chan struct{}) {
	return b.broker.Subscribe(
		ctx, topic,
		func(c *pubsub.BrokerContext[*Context, Message]) *Context {
			return &Context{
				brokerContext: c,
				sessionID:     sessionID,
			}
		},
		func(ctx context.Context, messages []Message) (ok bool) {
			rendered, err := b.triggerMsg(ctx, topic, sessionID, messages)
			if err != nil {
				b.logger.Error(errors.Wrapf(err, "msg handler on topic %s failed", topic))
				return false
			}
			return msgConsumer(ctx, rendered)
		},
	)
}

func (b *Broker) Serve(ctx context.Context) error {
	return b.broker.Serve(ctx, func(c *pubsub.BrokerContext[*Context, Message]) *Context {
		return &Context{
			brokerContext: c,
		}
	})
}

type Router interface {
	pubsub.Router[*Context]
	MSG(topic string, h HandlerFunc, m ...MiddlewareFunc) *Route
}
