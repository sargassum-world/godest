package actioncable

import (
	"context"
)

// Subscription represents the server end of an Action Cable subscription.
type Subscription struct {
	identifier string
	toClient   chan<- serverMessage
}

// Identifier returns the Action Cable subscription identifier.
func (s *Subscription) Identifier() string {
	return s.identifier
}

// SendText enqueues the string message for sending to the subscription's subscriber, blocking until
// the message is added to the queue.
func (s *Subscription) SendText(ctx context.Context, message string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.toClient <- newData(s.identifier, message):
		if err := ctx.Err(); err != nil {
			// Context was also canceled and it should have priority
			return err
		}
		// Go's runtime will randomize blocked goroutines sending to toClient to mitigate starvation,
		// but we'd need to implement round-robin scheduling if we wanted to guarantee a minimum
		// reservation on the capacity of toClient - e.g. like a clock sweep algorithm where the clock
		// advances when a semaphore indicates any subscription's outbox channel has data. We haven't
		// encountered any situation justifying the large extra complexity and synchronization overhead
		// needed for this.
		// TODO: enable configuring a subscription to drop data when its own outbox for s.toClient is
		// full - maybe we have a Subscription interface and LossySubscription vs. BlockingSubscription
		// types.
		return nil
	}
}

// SendBytes enqueues the string message for sending to the subscription's subscriber, blocking
// until the message is added to the queue.
func (s *Subscription) SendBytes(ctx context.Context, message []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.toClient <- newData(s.identifier, message):
		// Go's runtime will randomize blocked goroutines sending to toClient to mitigate starvation,
		// but we'd need to implement round-robin scheduling if we wanted to guarantee a minimum
		// reservation on the capacity of toClient - e.g. like a clock sweep algorithm where the clock
		// advances when a semaphore indicates any subscription's outbox channel has data. We haven't
		// encountered any situation justifying the large extra complexity and synchronization overhead
		// needed for this.
		if err := ctx.Err(); err != nil {
			// Context was also canceled and it should have priority
			return err
		}
		return nil
	}
}

// Close releases any resources associated with the subscription. The Subscription should not be
// used after it's closed.
func (s *Subscription) Close() {
	// We don't send a subscription rejection, because the @anycable/web client in the browser
	// expects no subscription rejection when it unsubscribes normally. Refer to
	// https://docs.anycable.io/misc/action_cable_protocol?id=subscriptions-amp-identifiers
	// Also, we leave the toClient channel open because other goroutines and Subscriptions should
	// still be able to send into it.
}
