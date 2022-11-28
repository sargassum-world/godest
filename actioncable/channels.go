package actioncable

import (
	"context"

	"github.com/pkg/errors"
)

// Channel represents a server-side Action Cable channel.
type Channel interface {
	// Subscribe handles an Action Cable subscribe command from the client with the provided
	// [Subscription].
	Subscribe(ctx context.Context, sub *Subscription) error
	// Perform handles an Action Cable action command from the client.
	Perform(data string) error
}

// ChannelFactory creates a Channel from an Action Cable subscription identifier.
type ChannelFactory func(identifier string) (Channel, error)

// ChannelDispatcher manages channels, channel subscriptions, and channel actions.
type ChannelDispatcher struct {
	factories map[string]ChannelFactory
	channels  map[string]Channel
	checkers  []IdentifierChecker
}

func NewChannelDispatcher(
	factories map[string]ChannelFactory, channels map[string]Channel, checkers ...IdentifierChecker,
) *ChannelDispatcher {
	return &ChannelDispatcher{
		factories: factories,
		channels:  channels,
		checkers:  checkers,
	}
}

// parseSubIdentifier parses and checks the subscription identifier.
func (d *ChannelDispatcher) parseSubIdentifier(identifier string) (channelName string, err error) {
	channelName, err = parseChannelName(identifier)
	if err != nil {
		return "", err
	}
	for _, checker := range d.checkers {
		if err = checker(identifier); err != nil {
			return "", errors.Wrapf(
				err, "subscription identifier for channel %s failed check", channelName,
			)
		}
	}
	return channelName, nil
}

// HandleSubscription associates the subscription to its corresponding channel and calls the
// channel's Subscribe method, first instantiating the channel if necessary.
func (d *ChannelDispatcher) HandleSubscription(ctx context.Context, sub *Subscription) error {
	if channel, ok := d.channels[sub.Identifier()]; ok {
		// The channel already exists, so we just subscribe to it again
		if err := channel.Subscribe(ctx, sub); err != nil {
			delete(d.channels, sub.Identifier())
			return errors.Wrapf(err, "couldn't re-subscribe to %s", sub.Identifier())
		}
		return nil
	}
	channelName, err := d.parseSubIdentifier(sub.Identifier())
	if err != nil {
		return err
	}

	// Create channel
	factory, ok := d.factories[channelName]
	if !ok {
		return errors.Errorf("unknown channel name %s", channelName)
	}
	channel, err := factory(sub.Identifier())
	if err != nil {
		return errors.Wrapf(
			err, "couldn't instantiate channel %s for subscription %s", channelName, sub.Identifier(),
		)
	}

	// Subscribe to the channel
	// The Subscribe method checks whether the subscription is possible, so we must check it before
	// storing the channel.
	if err = channel.Subscribe(ctx, sub); err != nil {
		return errors.Wrapf(err, "couldn't subscribe to %s", sub.Identifier())
	}
	d.channels[sub.Identifier()] = channel
	return nil
}

// HandleAction dispatches Action Cable action messages to the appropriate channels.
func (d *ChannelDispatcher) HandleAction(ctx context.Context, identifier, data string) error {
	channel, ok := d.channels[identifier]
	if !ok {
		return errors.Errorf("no preexisting subscription on %s", identifier)
	}
	return channel.Perform(data)
}
