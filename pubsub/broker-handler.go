package pubsub

import (
	"reflect"
	"runtime"

	"github.com/pkg/errors"
)

type Context interface {
	Method() string
	Topic() string
}

// Routing Handlers

type HandlerFunc[HandlerContext Context] func(c HandlerContext) error

func NotFoundHandler[HandlerContext Context](c HandlerContext) error {
	return errors.Errorf("handler not found for topic %s", c.Topic())
}

func MethodNotAllowedHandler[HandlerContext Context](c HandlerContext) error {
	return errors.Errorf("handler not found for method %s on topic %s", c.Method(), c.Topic())
}

func EmptyHandler[HandlerContext Context](c HandlerContext) error {
	return nil
}

func handlerName[HandlerContext Context](h HandlerFunc[HandlerContext]) string {
	// Copied from github.com/labstack/echo's handlerName function
	t := reflect.ValueOf(h).Type()
	if t.Kind() == reflect.Func {
		return runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	}
	return t.String()
}

// Middleware

type MiddlewareFunc[HandlerContext Context] func(
	next HandlerFunc[HandlerContext],
) HandlerFunc[HandlerContext]

func applyMiddleware[HandlerContext Context](
	h HandlerFunc[HandlerContext], middleware ...MiddlewareFunc[HandlerContext],
) HandlerFunc[HandlerContext] {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
