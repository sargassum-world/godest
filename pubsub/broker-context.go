package pubsub

import (
	"bytes"
	stdContext "context"
	"io"
)

type Context[Message any] interface {
	Context() stdContext.Context
	Method() string
	Topic() string
	SessionID() string
	Param(name string) string
	Hub() *Hub[[]Message]
	Publish(messages ...Message)
	Published() []Message
	MsgWriter() io.Writer
}

type context[Message any] struct {
	context   stdContext.Context
	method    string
	path      string
	pnames    []string
	pvalues   []string
	handler   HandlerFunc[Message]
	hub       *Hub[[]Message]
	topic     string
	sessionID string
	messages  []Message
	rendered  *bytes.Buffer
}

func (c *context[Message]) Context() stdContext.Context {
	return c.context
}

func (c *context[Message]) Method() string {
	return c.method
}

func (c *context[Message]) Topic() string {
	return c.topic
}

func (c *context[Message]) SessionID() string {
	return c.sessionID
}

func (c *context[Message]) Param(name string) string {
	// Copied from github.com/labstack/echo's context.Param method
	for i, n := range c.pnames {
		if i < len(c.pvalues) {
			if n == name {
				return c.pvalues[i]
			}
		}
	}
	return ""
}

func (c *context[Message]) Hub() *Hub[[]Message] {
	return c.hub
}

func (c *context[Message]) Publish(messages ...Message) {
	c.hub.Broadcast(c.topic, messages)
}

func (c *context[Message]) Published() []Message {
	return c.messages
}

func (c *context[Message]) MsgWriter() io.Writer {
	return c.rendered
}
