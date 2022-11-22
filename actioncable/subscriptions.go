package actioncable

type Subscription struct {
	identifier string
	toClient   chan<- serverMessage
}

func (s Subscription) Identifier() string {
	return s.identifier
}

func (s Subscription) Receive(message string) (ok bool) {
	select {
	default: // the receiver stopped listening
		return false
	case s.toClient <- newData(s.identifier, message):
		return true
	}
}

func (s Subscription) Close() {
	// We don't send a subscription rejection, because the @anycable/web client in t he browser
	// expects no subscription rejection when it unsubscribes normally. Refer to
	// https://docs.anycable.io/misc/action_cable_protocol?id=subscriptions-amp-identifiers
	// Also, we leave the toClient channel open because other goroutines and Subscriptions should
	// still be able to send into it.
}
