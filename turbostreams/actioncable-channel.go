package turbostreams

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/actioncable"
	"github.com/sargassum-world/godest/pubsub"
)

// ChannelName is the name of the Action Cable channel for Turbo Streams.
const ChannelName = "Turbo::StreamsChannel"

// subscriber creates a subscription for the channel, to integrate [Channel] with [Broker].
type subscriber func(
	ctx context.Context, topic, sessionID string,
	msgConsumer func(ctx context.Context, rendered string) error,
) (finished <-chan struct{})

// Channel represents an Action Cable channel for a Turbo Streams stream.
type Channel struct {
	identifier string
	streamName string
	h          *pubsub.Hub[[]Message]
	subscriber subscriber
	sessionID  string
}

// parseStreamName parses the Turbo Streams stream name from the Action Cable subscription
// identifier.
func parseStreamName(identifier string) (string, error) {
	var i struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(identifier), &i); err != nil {
		return "", errors.Wrap(
			err, "couldn't parse stream name from action cable subscription identifier",
		)
	}
	return i.Name, nil
}

// NewChannel checks the identifier with the specified checkers and returns a new Channel instance.
func NewChannel(
	identifier string, h *pubsub.Hub[[]Message], subscriber subscriber, sessionID string,
	checkers []actioncable.IdentifierChecker,
) (*Channel, error) {
	name, err := parseStreamName(identifier)
	if err != nil {
		return nil, err
	}
	for _, checker := range checkers {
		if err := checker(identifier); err != nil {
			return nil, errors.Wrap(err, "action cable subscription identifier failed checks")
		}
	}
	return &Channel{
		identifier: identifier,
		streamName: name,
		h:          h,
		subscriber: subscriber,
		sessionID:  sessionID,
	}, nil
}

// Subscribe handles an Action Cable subscribe command from the client with the provided
// [actioncable.Subscription].
func (c *Channel) Subscribe(ctx context.Context, sub *actioncable.Subscription) error {
	if sub.Identifier() != c.identifier {
		return errors.Errorf(
			"channel identifier %+v does not match subscription identifier %+v",
			c.identifier, sub.Identifier(),
		)
	}

	finished := c.subscriber(
		ctx, c.streamName, c.sessionID, func(ctx context.Context, rendered string) error {
			return sub.SendText(rendered)
		},
	)
	go func() {
		<-finished
		sub.Close()
	}()
	return nil
}

// Perform handles an Action Cable action command from the client.
func (c *Channel) Perform(data string) error {
	return errors.New("turbo streams channel cannot perform any actions")
}

// NewChannelFactory creates an [actioncable.ChannelFactory] for Turbo Streams to create channels
// for different Turbo Streams streams as needed.
func NewChannelFactory(
	b *Broker, sessionID string, checkers ...actioncable.IdentifierChecker,
) actioncable.ChannelFactory {
	return func(identifier string) (actioncable.Channel, error) {
		return NewChannel(identifier, b.Hub(), b.Subscribe, sessionID, checkers)
	}
}
