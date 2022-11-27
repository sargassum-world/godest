package pubsub

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
)

// Broker is the top-level pub-sub framework for routing pub-sub events to handlers. Analogous to
// Echo's Echo.
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

// NewBroker creates an instance of [Broker].
func NewBroker[HandlerContext Context, Message any](logger Logger) *Broker[HandlerContext, Message] {
	changes := make(chan BroadcastingChange)
	hub := NewHub[[]Message](changes, logger)
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

// Hub returns the associated pub-sub [Hub].
func (b *Broker[HandlerContext, Message]) Hub() *Hub[[]Message] {
	return b.hub
}

// Handler Registration

// Add registers a new route for a pub-sub broker event method and topic with matching handler in
// the router, with optional route-level middleware. Analogous to Echo's Echo.Add.
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

// PUB registers a new PUB route for a topic with matching handler in the router, with optional
// route-level middleware. Refer to [Broker.Serve] for details on how PUB handlers are used.
func (b *Broker[HandlerContext, Message]) PUB(
	topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext],
) *Route {
	return b.Add(MethodPub, topic, h, m...)
}

// SUB registers a new SUB route for a topic with matching handler in the router, with optional
// route-level middleware. Refer to [Broker.Subscribe] for details on how SUB handlers are used.
func (b *Broker[HandlerContext, Message]) SUB(
	topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext],
) *Route {
	return b.Add(MethodSub, topic, h, m...)
}

// UNSUB registers a new UNSUB route for a topic with matching handler in the router, with optional
// route-level middleware. Refer to [Broker.Subscribe] for details on how UNSUB handlers are used.
func (b *Broker[HandlerContext, Message]) UNSUB(
	topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext],
) *Route {
	return b.Add(MethodUnsub, topic, h, m...)
}

// Middleware

// Use adds middleware to the chain which is run after the router. Analogous to Echo's Echo.Use.
func (b *Broker[HandlerContext, Message]) Use(middleware ...MiddlewareFunc[HandlerContext]) {
	b.middleware = append(b.middleware, middleware...)
}

// GetHandler returns the handler associated in the router with the method and topic,
// with middleware (specified by [Broker.Use]) applied afterwards, and with the provided router
// context modified based on routing results. This is useful in combination with
// [Broker.NewBrokerContext] for triggering handlers for custom pub-sub broker event methods.
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

// NewBrokerContext creates a new pub-sub broker event context. This is useful in combination with
// [Broker.GetHandler] for triggering handlers for custom pub-sub broker event methods.
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

// HandlerContextMaker is a function for creating a HandlerContext instance from a [BrokerContext]
// instance created by [Broker.NewBrokerContext].
type HandlerContextMaker[HandlerContext Context, Message any] func(
	c *BrokerContext[HandlerContext, Message],
) HandlerContext

// TriggerSub looks up and runs the SUB handler associated with the topic.
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

// TriggerUnsub looks up and runs the UNSUB handler associated with the topic.
func (b *Broker[HandlerContext, Message]) TriggerUnsub(
	ctx context.Context, topic string, hc HandlerContextMaker[HandlerContext, Message],
) {
	c := b.NewBrokerContext(ctx, MethodUnsub, topic)
	h := b.GetHandler(MethodUnsub, topic, c.routerContext)
	if err := h(hc(c)); err != nil && !errors.Is(err, context.Canceled) {
		b.logger.Error(errors.Wrapf(err, "couldn't trigger unsub handler on topic %s", topic))
	}
}

// TriggerPub looks up and launches a goroutine running the PUB handler associated with the topic.
// It returns an error if the handler for that topic already started but has not yet been canceled
// by a call to [Broker.CancelPub]. Useful for writing alternatives to [Broker.Serve].
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

// CancelPub cancels the PUB handler goroutine for the topic started by [Broker.TriggerPub].
// Useful for writing alternatives to [Broker.Serve].
func (b *Broker[HandlerContext, Message]) CancelPub(topic string) {
	if canceller, ok := b.pubCancelers[topic]; ok {
		canceller()
		delete(b.pubCancelers, topic)
	}
}

// Subscribe runs the SUB handler for the topic and, if it does not produce an error, adds a
// subscription to the broker's Hub with a callback function to handle messages broadcast over the
// broker's Hub. When the context is canceled, the UNSUB handler is run. Any messages published on
// the broker's Hub (e.g. messages broadcast from [Context.Publish], [Context.Broadcast], or
// [Hub.Broadcast]) will be passed to the broadcast handler callback function.
func (b *Broker[HandlerContext, Message]) Subscribe(
	ctx context.Context, topic string, hc HandlerContextMaker[HandlerContext, Message],
	broadcastHandler func(ctx context.Context, messages []Message) error,
) (finished <-chan struct{}) {
	if err := b.TriggerSub(ctx, topic, hc); err != nil {
		return nil
	}
	cctx, cancel := context.WithCancel(ctx)
	removed := b.hub.Subscribe(cctx, topic, func(messages []Message) error {
		if err := cctx.Err(); err != nil {
			return err
		}
		if err := broadcastHandler(cctx, messages); err != nil {
			cancel()
			return err
		}
		return nil
	})
	go func() {
		<-removed
		b.TriggerUnsub(cctx, topic, hc)
	}()
	return removed
}

// Managed Publishing

// Serve launches and cancels PUB handlers based on the appearance and disappearance of
// subscriptions for the PUB handlers' corresponding topics. The PUB handler for a topic is started
// in a goroutine when a new subscription is added to the broker (or to the broker's Hub) on a topic
// which previously did not have associated subscriptions; and its context is canceled upon removal
// of the only remaining subscription for the topic. This way, exactly one instance of the PUB
// handler for a topic is run exactly when there is at least one subscriber on that topic.
// The Broker should not be used after the Serve method finishes running.
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

// Router is the subset of [Broker] methods for adding handlers to routes.
type Router[HandlerContext Context] interface {
	PUB(topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext]) *Route
	SUB(topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext]) *Route
	UNSUB(topic string, h HandlerFunc[HandlerContext], m ...MiddlewareFunc[HandlerContext]) *Route
}
