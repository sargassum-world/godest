package pubsub

import (
	"reflect"
	"runtime"

	"github.com/pkg/errors"
)

// Context is the type constraint for all broker handler contexts, representing the context of the
// current pub-sub broker event.
type Context interface {
	// Method returns the handler method for the broker event, such as subscription. Analogous to
	// Echo's Context.Request().Method.
	Method() string
	// Method returns the handler topic for the broker event. Analogous to Echo's
	// Context.Request().URL.RequestURI method.
	Topic() string
}

// Routing Handlers

// HandlerFunc defines a function to handle pub-sub events. Analogous to Echo's HandlerFunc.
type HandlerFunc[HandlerContext Context] func(c HandlerContext) error

// NotFoundHandler is the fallback handler used for handling pub-sub broker events on topics lacking
// a matching broker handler. Analogous to Echo's NotFoundHandler.
func NotFoundHandler[HandlerContext Context](c HandlerContext) error {
	return errors.Errorf("handler not found for topic %s", c.Topic())
}

// MethodNotAllowedHandler is the fallback handler used for handling pub-sub broker events with
// methods lacking a matching broker handler. Analogous to Echo's MethodNotAllowedHandler.
func MethodNotAllowedHandler[HandlerContext Context](c HandlerContext) error {
	return errors.Errorf("handler not found for method %s on topic %s", c.Method(), c.Topic())
}

// handlerName returns the name of the broker handler.
func handlerName[HandlerContext Context](h HandlerFunc[HandlerContext]) string {
	// Copied from github.com/labstack/echo's handlerName function
	t := reflect.ValueOf(h).Type()
	if t.Kind() == reflect.Func {
		return runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	}
	return t.String()
}

// Middleware

// MiddlewareFunc defines a function to process middleware. Analogous to Echo's MiddlewareFunc.
type MiddlewareFunc[HandlerContext Context] func(
	next HandlerFunc[HandlerContext],
) HandlerFunc[HandlerContext]

// applyMiddleware wraps the handler in the middlewares to create the returned [HandlerFunc].
func applyMiddleware[HandlerContext Context](
	h HandlerFunc[HandlerContext], middleware ...MiddlewareFunc[HandlerContext],
) HandlerFunc[HandlerContext] {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
