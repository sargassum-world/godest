package actioncable

import (
	"github.com/pkg/errors"
)

// Subscription represents the server end of an Action Cable subscription.
type Subscription struct {
	identifier string
	toClient   chan<- serverMessage
}

// Identifier returns the Action Cable subscription identifier.
func (s Subscription) Identifier() string {
	return s.identifier
}

// Send enqueues the message for sending to the subscription's subscriber, blocking until the
// message is added to the queue.
func (s Subscription) Send(message string) error {
	select {
	default:
		return errors.New("receiver stopped listening")
	case s.toClient <- newData(s.identifier, message):
		// TODO: enable round-robin scheduling of message receiving between subscriptions on a
		// connection, so that every subscription is guaranteed some minimum use of the connection.
		// Maybe each Subscription can be given its own unique toClient, and then the connection cycles
		// through the per-subscription toClient channels according to some strategy. We would use a
		// semaphore (provided by golang.org/x/sync) to make the consumer of toClient channels sleep
		// until a channel is ready for consumption, and then we'd go through the list of toClient
		// channels according to our strategy.
		// Since the Conn isn't aware of priorities, we can just do a simple round-robin to give every
		// channel equal priority on a connection.
		// TODO: enable configuring a subscription to drop data when its own outbox for s.toClient is
		// full - maybe we have a Subscription interface and LossySubscription vs. BlockingSubscription
		// types.
		return nil
	}
}

// Close releases any resources associated with the subscription. The Subscription should not be
// used after it's closed.
func (s Subscription) Close() {
	// We don't send a subscription rejection, because the @anycable/web client in t he browser
	// expects no subscription rejection when it unsubscribes normally. Refer to
	// https://docs.anycable.io/misc/action_cable_protocol?id=subscriptions-amp-identifiers
	// Also, we leave the toClient channel open because other goroutines and Subscriptions should
	// still be able to send into it.
	// TODO: close the channel once we give each subscription its own channel
}
