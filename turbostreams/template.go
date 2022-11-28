// Package turbostreams provides server-side support for sending Hotwired Turbo Streams in POST
// responses as well as over Action Cable.
package turbostreams

import (
	_ "embed"
)

// Action is the Turbo Stream action.
type Action string

// Standard Turbo Stream actions.
const (
	ActionAppend  Action = "append"
	ActionPrepend Action = "prepend"
	ActionReplace Action = "replace"
	ActionUpdate  Action = "update"
	ActionRemove  Action = "remove"
	ActionBefore  Action = "before"
	ActionAfter   Action = "after"
)

// Message represents a Turbo Stream message which can be rendered to a string using the
// specified template.
type Message struct {
	Action   Action
	Target   string
	Targets  string
	Template string
	Data     interface{}
}

// Template is a Go HTTP template string for rendering [Message] instances as HTML.
//
//go:embed streams.partial.tmpl
var Template string
