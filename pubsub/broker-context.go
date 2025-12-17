package pubsub

import (
	"context"
	"net/url"

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

// BrokerContext represents the broker's portion of the context of the current pub-sub broker event.
// Usually should be an embedded type in HandlerContext, which is analogous to Echo's Context.
type BrokerContext[HandlerContext Context, Message any] struct {
	context context.Context
	logger  Logger

	method string
	topic  string

	routerContext *RouterContext[HandlerContext]
	hub           *Hub[[]Message]
}

// Context returns the associated cancellation [context.Context]. Analogous to Echo's
// Context.Request().Context.
func (c *BrokerContext[HandlerContext, Message]) Context() context.Context {
	return c.context
}

// Logger returns the associated [Logger] instance. Analogous to Echo's Context.Logger.
func (c *BrokerContext[HandlerContext, Message]) Logger() Logger {
	return c.logger
}

// Method returns the associated pub-sub broker event method. Analogous to Echo's
// Context.Request().Method.
func (c *BrokerContext[HandlerContext, Message]) Method() string {
	return c.method
}

// Topic returns the associated pub-sub broker event topic. Analogous to Echo's
// Context.Request().URL.RequestURI.
func (c *BrokerContext[HandlerContext, Message]) Topic() string {
	return c.topic
}

// RouterContext returns the associated pub-sub broker routing context.
func (c *BrokerContext[HandlerContext, Message]) RouterContext() *RouterContext[HandlerContext] {
	return c.routerContext
}

// Path returns the registered path for the handler. Analogous to Echo's Context.Path.
func (c *BrokerContext[HandlerContext, Message]) Path() string {
	return c.routerContext.path
}

// Param returns the topic's path parameter value by name. Analogous to Echo's Context.Param.
func (c *BrokerContext[HandlerContext, Message]) Param(name string) string {
	return c.routerContext.Param(name)
}

// ParamNames returns the path parameter names. Analogous to Echo's Context.ParamNames.
func (c *BrokerContext[HandlerContext, Message]) ParamNames() []string {
	return c.routerContext.ParamNames()
}

// ParamValues returns the path parameter values. Analogous to Echo's Context.ParamValues.
func (c *BrokerContext[HandlerContext, Message]) ParamValues() []string {
	return c.routerContext.ParamValues()
}

// QueryParams returns the query parameters as [url.Values], if the topic can be parsed as a request
// URI. Analogous to Echo's Context.QueryParams.
func (c *BrokerContext[HandlerContext, Message]) QueryParams() (url.Values, error) {
	topic, err := url.ParseRequestURI(c.Topic())
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't parse topic %s as request URI", c.Topic())
	}
	return topic.Query(), nil
}

// QueryParams returns the first value of the specified query parameter, if the topic can be parsed
// as a request URI. Analogous to Echo's Context.QueryParam.
func (c *BrokerContext[HandlerContext, Message]) QueryParam(name string) (string, error) {
	values, err := c.QueryParams()
	if err != nil {
		return "", err
	}
	return values.Get(name), nil
}

// Hub returns the associated broker's pub-sub [Hub].
func (c *BrokerContext[HandlerContext, Message]) Hub() *Hub[[]Message] {
	return c.hub
}

// Publish broadcasts the messages over the associated broker's pub-sub [Hub], on the same topic
// as the BrokerContext itself.
func (c *BrokerContext[HandlerContext, Message]) Publish(messages ...Message) {
	c.hub.Broadcast(c.topic, messages)
}

// Broadcast broadcasts the messages over the associated broker's pub-sub [Hub], on the specified
// topic.
func (c *BrokerContext[HandlerContext, Message]) Broadcast(topic string, messages ...Message) {
	c.hub.Broadcast(topic, messages)
}
