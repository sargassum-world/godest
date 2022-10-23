package pubsub

import (
	stdContext "context"
	"reflect"
	"runtime"

	"github.com/pkg/errors"
)

// Methods

const (
	MethodPub   = "PUB"
	MethodSub   = "SUB"
	MethodUnsub = "UNSUB"
	MethodMsg   = "MSG"
)

// Handlers

type HandlerFunc[Message any] func(c Context[Message]) error

func NotFoundHandler[Message any](c Context[Message]) error {
	return errors.Errorf("handler not found for topic %s", c.Topic())
}

func EmptyHandler[Message any](c Context[Message]) error {
	return nil
}

type (
	SubHandler              func(ctx stdContext.Context, topic string) error
	UnsubHandler            func(ctx stdContext.Context, topic string)
	MsgHandler[Message any] func(
		ctx stdContext.Context, topic string, messages []Message,
	) (result string, err error)
)

type methodHandler[Message any] struct {
	pub   HandlerFunc[Message]
	sub   HandlerFunc[Message]
	unsub HandlerFunc[Message]
	msg   HandlerFunc[Message]
}

func (m *methodHandler[Message]) isHandler() bool {
	return m.pub != nil || m.sub != nil || m.unsub != nil || m.msg != nil
}

func handlerName[Message any](h HandlerFunc[Message]) string {
	// Copied from github.com/labstack/echo's handlerName function
	t := reflect.ValueOf(h).Type()
	if t.Kind() == reflect.Func {
		return runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	}
	return t.String()
}

// Middleware

type MiddlewareFunc[Message any] func(next HandlerFunc[Message]) HandlerFunc[Message]

func applyMiddleware[Message any](
	h HandlerFunc[Message], middleware ...MiddlewareFunc[Message],
) HandlerFunc[Message] {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
