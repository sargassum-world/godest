package pubsub

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
)

// Context

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

type BrokerContext[HandlerContext Context, Message any] struct {
	context context.Context
	logger  Logger

	method string
	topic  string

	routerContext *RouterContext[HandlerContext]
	hub           *Hub[[]Message]
}

func (c *BrokerContext[HandlerContext, Message]) Context() context.Context {
	return c.context
}

func (c *BrokerContext[HandlerContext, Message]) Logger() Logger {
	return c.logger
}

func (c *BrokerContext[HandlerContext, Message]) Method() string {
	return c.method
}

func (c *BrokerContext[HandlerContext, Message]) Topic() string {
	return c.topic
}

func (c *BrokerContext[HandlerContext, Message]) RouterContext() *RouterContext[HandlerContext] {
	return c.routerContext
}

func (c *BrokerContext[HandlerContext, Message]) Path() string {
	return c.routerContext.path
}

func (c *BrokerContext[HandlerContext, Message]) Param(name string) string {
	return c.routerContext.Param(name)
}

func (c *BrokerContext[HandlerContext, Message]) ParamNames() []string {
	return c.routerContext.ParamNames()
}

func (c *BrokerContext[HandlerContext, Message]) ParamValues() []string {
	return c.routerContext.ParamValues()
}

func (c *BrokerContext[HandlerContext, Message]) Query() (url.Values, error) {
	topic, err := url.ParseRequestURI(c.Topic())
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't parse topic %s as request URI", c.Topic())
	}
	return topic.Query(), nil
}

func (c *BrokerContext[HandlerContext, Message]) Hub() *Hub[[]Message] {
	return c.hub
}

func (c *BrokerContext[HandlerContext, Message]) Publish(messages ...Message) {
	c.hub.Broadcast(c.topic, messages)
}

// Broker

type Broker[HandlerContext Context, Message any] struct {
	hub      *Hub[[]Message]
	router   *HandlerRouter[HandlerContext]
	maxParam *int
	logger   Logger

	middleware []MiddlewareFunc[HandlerContext]

	// This is not guarded by a mutex because it's only used by a single goroutine
	pubCancelers map[string]context.CancelFunc
	changes      <-chan BroadcastingChange
}

func NewBroker[HandlerContext Context, Message any](logger Logger) *Broker[HandlerContext, Message] {
	changes := make(chan BroadcastingChange)
	hub := NewHub[[]Message](changes)
	maxParam := new(int)
	return &Broker[HandlerContext, Message]{
		hub:          hub,
		router:       NewHandlerRouter[HandlerContext](maxParam),
		maxParam:     maxParam,
		logger:       logger,
		pubCancelers: make(map[string]context.CancelFunc),
		changes:      changes,
	}
}

func (b *Broker[HandlerContext, Message]) Hub() *Hub[[]Message] {
	return b.hub
}

// Handler Registration

func (b *Broker[HandlerContext, Message]) Add(
	method, topic string, handler HandlerFunc[HandlerContext],
	middleware ...MiddlewareFunc[HandlerContext],
) *Route {
	// Copied from github.com/labstack/echo's Echo.add method
	name := handlerName(handler)
	router := b.router
	router.Add(method, topic, func(c HandlerContext) error {
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

func (b *Broker[HandlerContext, Message]) PUB(
	topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext],
) *Route {
	return b.Add(MethodPub, topic, h, m...)
}

func (b *Broker[HandlerContext, Message]) SUB(
	topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext],
) *Route {
	return b.Add(MethodSub, topic, h, m...)
}

func (b *Broker[HandlerContext, Message]) UNSUB(
	topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext],
) *Route {
	return b.Add(MethodUnsub, topic, h, m...)
}

// Middleware

func (b *Broker[HandlerContext, Message]) Use(middleware ...MiddlewareFunc[HandlerContext]) {
	b.middleware = append(b.middleware, middleware...)
}

func (b *Broker[HandlerContext, Message]) GetHandler(
	method string, topic string, c *RouterContext[HandlerContext],
) HandlerFunc[HandlerContext] {
	if u, err := url.ParseRequestURI(topic); err == nil {
		b.router.Find(method, u.Path, c)
	} else {
		b.router.Find(method, topic, c)
	}
	return applyMiddleware(c.handler, b.middleware...)
}

// Handler Triggering

func (b *Broker[HandlerContext, Message]) NewBrokerContext(
	ctx context.Context, method, topic string,
) *BrokerContext[HandlerContext, Message] {
	return &BrokerContext[HandlerContext, Message]{
		context: ctx,
		routerContext: &RouterContext[HandlerContext]{
			pnames:  make([]string, *b.maxParam),
			pvalues: make([]string, *b.maxParam),
			handler: NotFoundHandler[HandlerContext],
		},
		logger: b.logger,
		method: method,
		topic:  topic,
		hub:    b.hub,
	}
}

type HandlerContextMaker[HandlerContext Context, Message any] func(
	c *BrokerContext[HandlerContext, Message],
) HandlerContext

func (b *Broker[HandlerContext, Message]) TriggerSub(
	ctx context.Context, topic string, hc HandlerContextMaker[HandlerContext, Message],
) error {
	c := b.NewBrokerContext(ctx, MethodSub, topic)
	h := b.GetHandler(MethodSub, topic, c.routerContext)
	if err := h(hc(c)); err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error(errors.Wrapf(err, "couldn't trigger sub handler on topic %s", topic))
		return err
	}
	return nil
}

func (b *Broker[HandlerContext, Message]) TriggerUnsub(
	ctx context.Context, topic string, hc HandlerContextMaker[HandlerContext, Message],
) {
	c := b.NewBrokerContext(ctx, MethodUnsub, topic)
	h := b.GetHandler(MethodUnsub, topic, c.routerContext)
	if err := h(hc(c)); err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error(errors.Wrapf(err, "couldn't trigger unsub handler on topic %s", topic))
	}
}

func (b *Broker[HandlerContext, Message]) TriggerPub(
	ctx context.Context, topic string, hc HandlerContextMaker[HandlerContext, Message],
) error {
	if _, ok := b.pubCancelers[topic]; ok {
		return errors.Errorf("existing pub handler for topic %s has not yet been canceled", topic)
	}

	ctx, canceler := context.WithCancel(ctx)
	b.pubCancelers[topic] = canceler
	c := b.NewBrokerContext(ctx, MethodPub, topic)
	h := b.GetHandler(MethodPub, topic, c.routerContext)
	go func() {
		if err := h(hc(c)); err != nil && !errors.Is(err, context.Canceled) {
			b.logger.Error(errors.Wrapf(err, "pub handler for topic %s failed", topic))
		}
	}()
	return nil
}

func (b *Broker[HandlerContext, Message]) CancelPub(topic string) {
	if canceller, ok := b.pubCancelers[topic]; ok {
		canceller()
		delete(b.pubCancelers, topic)
	}
}

func (b *Broker[HandlerContext, Message]) Subscribe(
	ctx context.Context, topic string, hc HandlerContextMaker[HandlerContext, Message],
	broadcastHandler func(ctx context.Context, messages []Message) (ok bool),
) (unsubscriber func(), finished <-chan struct{}) {
	if err := b.TriggerSub(ctx, topic, hc); err != nil {
		return nil, nil // since subscribing isn't possible/authorized, reject the subscription
	}
	ctx, cancel := context.WithCancel(ctx)
	unsub, removed := b.hub.Subscribe(topic, func(messages []Message) (ok bool) {
		if ctx.Err() != nil {
			return false
		}
		if ok := broadcastHandler(ctx, messages); !ok {
			cancel()
			return false
		}
		return true
	})
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			break
		case <-removed:
			break
		}
		cancel()
		unsub()
		b.TriggerUnsub(ctx, topic, hc)
		close(done)
	}()
	return cancel, done
}

// Managed Publishing

func (b *Broker[HandlerContext, Message]) Serve(
	ctx context.Context, hc HandlerContextMaker[HandlerContext, Message],
) error {
	go func() {
		<-ctx.Done()
		b.hub.Close()
	}()
	for change := range b.changes {
		for _, topic := range change.Added {
			if _, ok := b.pubCancelers[topic]; ok {
				continue
			}
			if err := b.TriggerPub(ctx, topic, hc); err != nil {
				b.logger.Errorf("couldn't automatically trigger pub handler for topic %s", topic)
			}
		}
		for _, topic := range change.Removed {
			b.CancelPub(topic)
		}
	}
	return ctx.Err()
}

// Router Interface

type Router[HandlerContext Context] interface {
	PUB(topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext]) *Route
	SUB(topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext]) *Route
	UNSUB(topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext]) *Route
}
