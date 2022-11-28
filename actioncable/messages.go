package actioncable

import (
	"fmt"
	"time"
)

// Client Messages

// Client-to-server commands.
const (
	subscribeCommand   = "subscribe"
	unsubscribeCommand = "unsubscribe"
	actionCommand      = "message"
)

// clientMessage represents generic client-to-server messages.
type clientMessage struct {
	Command    string `json:"command"`
	Identifier string `json:"identifier"`
	Data       string `json:"data,omitempty"`
}

// Server Messages

// serverMessage represents generic server-to-client messages.
type serverMessage struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Message    any    `json:"message,omitempty"`
}

// newWelcome creates an Action Cable welcome message.
func newWelcome() serverMessage {
	return serverMessage{
		Type: "welcome",
	}
}

// newWelcome creates an Action Cable ping message.
func newPing(t time.Time) serverMessage {
	return serverMessage{
		Type:    "ping",
		Message: fmt.Sprintf("%d", t.Unix()),
	}
}

// newSubscriptionConfirmation creates an Action Cable confirm_subscription message.
func newSubscriptionConfirmation(identifier string) serverMessage {
	return serverMessage{
		Type:       "confirm_subscription",
		Identifier: identifier,
	}
}

// newSubscriptionRejection creates an Action Cable reject_subscription message.
func newSubscriptionRejection(identifier string) serverMessage {
	return serverMessage{
		Type:       "reject_subscription",
		Identifier: identifier,
	}
}

type DataPayload interface {
	string | []byte
}

// newData creates an Action Cable data message.
func newData[Payload DataPayload](identifier string, message Payload) serverMessage {
	return serverMessage{
		Identifier: identifier,
		Message:    message,
	}
}

// disconnectMessage represents a server-to-client disconnect message.
type disconnectMessage struct {
	Type      string `json:"type"`
	Reason    string `json:"reason,omitempty"`
	Reconnect bool   `json:"reconnect,omitempty"`
}

// newDisconnect creates an Action Cable disconnect message.
func newDisconnect(reason string, allowReconnect bool) disconnectMessage {
	return disconnectMessage{
		Type:      "disconnect",
		Reason:    reason,
		Reconnect: allowReconnect,
	}
}
