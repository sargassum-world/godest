package pubsub

import (
	"bytes"
	stdContext "context"

	"github.com/pkg/errors"
)

// Logger is a reduced interface for loggers.
type Logger interface {
	Print(i ...interface{})
	Printf(format string, args ...interface{})
	Debug(i ...interface{})
	Debugf(format string, args ...interface{})
	Info(i ...interface{})
	Infof(format string, args ...interface{})
	Warn(i ...interface{})
	Warnf(format string, args ...interface{})
	Error(i ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(i ...interface{})
	Fatalf(format string, args ...interface{})
	Panic(i ...interface{})
	Panicf(format string, args ...interface{})
}

type Router[Message any] interface {
	PUB(topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message]) *Route
	SUB(topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message]) *Route
	UNSUB(topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message]) *Route
	MSG(topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message]) *Route
}

type Broker[Message any] struct {
	hub      *Hub[[]Message]
	router   *router[Message]
	maxParam *int
	logger   Logger

	middleware []MiddlewareFunc[Message]

	// This is not guarded by a mutex because it's only used by a single goroutine
	pubCancellers map[string]stdContext.CancelFunc
	changes       <-chan BroadcastingChange
}

func NewBroker[Message any](logger Logger) *Broker[Message] {
	changes := make(chan BroadcastingChange)
	hub := NewHub[[]Message](changes)
	b := &Broker[Message]{
		hub:           hub,
		changes:       changes,
		pubCancellers: make(map[string]stdContext.CancelFunc),
		maxParam:      new(int),
		logger:        logger,
	}
	b.router = newRouter(b)
	return b
}

func (b *Broker[Message]) Hub() *Hub[[]Message] {
	return b.hub
}

// Handler Registration

func (b *Broker[Message]) Add(
	method, topic string, handler HandlerFunc[Message], middleware ...MiddlewareFunc[Message],
) *Route {
	// Copied from github.com/labstack/echo's Echo.add method
	name := handlerName(handler)
	router := b.router
	router.Add(method, topic, func(c Context[Message]) error {
		h := applyMiddleware(handler, middleware...)
		return h(c)
	})
	r := &Route{
		Method: method,
		Path:   topic,
		Name:   name,
	}
	b.router.routes[method+topic] = r
	return r
}

func (b *Broker[Message]) PUB(
	topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message],
) *Route {
	return b.Add(MethodPub, topic, h, m...)
}

func (b *Broker[Message]) SUB(
	topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message],
) *Route {
	return b.Add(MethodSub, topic, h, m...)
}

func (b *Broker[Message]) UNSUB(
	topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message],
) *Route {
	return b.Add(MethodUnsub, topic, h, m...)
}

func (b *Broker[Message]) MSG(
	topic string, h HandlerFunc[Message], m ...MiddlewareFunc[Message],
) *Route {
	return b.Add(MethodMsg, topic, h, m...)
}

// Middleware

func (b *Broker[Message]) Use(middleware ...MiddlewareFunc[Message]) {
	b.middleware = append(b.middleware, middleware...)
}

func (b *Broker[Message]) getHandler(
	method string, topic string, c *context[Message],
) HandlerFunc[Message] {
	b.router.Find(method, topic, c)
	return applyMiddleware(c.handler, b.middleware...)
}

// Handlers

func (b *Broker[Message]) newContext(ctx stdContext.Context, topic string) *context[Message] {
	return &context[Message]{
		context: ctx,
		pvalues: make([]string, *b.maxParam),
		handler: NotFoundHandler[Message],
		hub:     b.hub,
		topic:   topic,
	}
}

func (b *Broker[Message]) SubHandler(sessionID string) SubHandler {
	return func(ctx stdContext.Context, topic string) error {
		c := b.newContext(ctx, topic)
		c.method = MethodSub
		c.sessionID = sessionID
		h := b.getHandler(MethodSub, topic, c)
		err := errors.Wrapf(h(c), "couldn't handle subscribe on topic %s", topic)
		if err != nil && !errors.Is(err, stdContext.Canceled) {
			b.logger.Error(err)
		}
		return err
	}
}

func (b *Broker[Message]) UnsubHandler(sessionID string) UnsubHandler {
	return func(ctx stdContext.Context, topic string) {
		c := b.newContext(ctx, topic)
		c.method = MethodUnsub
		c.sessionID = sessionID
		h := b.getHandler(MethodUnsub, topic, c)
		err := errors.Wrapf(h(c), "couldn't handle unsubscribe on topic %s", topic)
		if err != nil && !errors.Is(err, stdContext.Canceled) {
			b.logger.Error(err)
		}
	}
}

func (b *Broker[Message]) MsgHandler(sessionID string) MsgHandler[Message] {
	return func(ctx stdContext.Context, topic string, messages []Message) (result string, err error) {
		c := b.newContext(ctx, topic)
		c.method = MethodMsg
		c.sessionID = sessionID
		c.messages = messages
		c.rendered = &bytes.Buffer{}
		h := b.getHandler(MethodMsg, topic, c)
		err = errors.Wrapf(h(c), "couldn't handle message post-processing on topic %s", topic)
		if err != nil && !errors.Is(err, stdContext.Canceled) {
			b.logger.Error(err)
			return "", err
		}
		return c.rendered.String(), nil
	}
}

// Managed Publishing

func (b *Broker[Message]) startPub(ctx stdContext.Context, topic string) {
	ctx, canceler := stdContext.WithCancel(ctx)
	c := b.newContext(ctx, topic)
	c.method = MethodPub
	b.pubCancellers[topic] = canceler
	h := b.getHandler(MethodPub, topic, c)
	go func() {
		err := h(c)
		if err != nil && !errors.Is(err, stdContext.Canceled) {
			b.logger.Error(err)
		}
	}()
}

func (b *Broker[Message]) cancelPub(topic string) {
	if canceller, ok := b.pubCancellers[topic]; ok {
		canceller()
		delete(b.pubCancellers, topic)
	}
}

func (b *Broker[Message]) Serve(ctx stdContext.Context) error {
	go func() {
		<-ctx.Done()
		b.hub.Close()
	}()
	for change := range b.changes {
		for _, topic := range change.Added {
			if _, ok := b.pubCancellers[topic]; ok {
				continue
			}
			b.startPub(ctx, topic)
		}
		for _, topic := range change.Removed {
			b.cancelPub(topic)
		}
	}
	return ctx.Err()
}
