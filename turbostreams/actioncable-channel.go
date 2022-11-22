package turbostreams

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/sargassum-world/godest/actioncable"
	"github.com/sargassum-world/godest/pubsub"
)

const ChannelName = "Turbo::StreamsChannel"

type subscriber func(
	ctx context.Context, topic, sessionID string,
	msgConsumer func(ctx context.Context, rendered string) (ok bool),
) (unsubscriber func(), finished <-chan struct{})

type Channel struct {
	identifier string
	streamName string
	h          *pubsub.Hub[[]Message]
	subscriber subscriber
	sessionID  string
}

func parseStreamName(identifier string) (string, error) {
	var i struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(identifier), &i); err != nil {
		return "", errors.Wrap(err, "couldn't parse stream name from identifier")
	}
	return i.Name, nil
}

func NewChannel(
	identifier string, h *pubsub.Hub[[]Message], subscriber subscriber, sessionID string,
	checkers ...actioncable.IdentifierChecker,
) (*Channel, error) {
	name, err := parseStreamName(identifier)
	if err != nil {
		return nil, err
	}
	for _, checker := range checkers {
		if err := checker(identifier); err != nil {
			return nil, errors.Wrap(err, "stream identifier failed checks")
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

func (c *Channel) Subscribe(
	ctx context.Context, sub actioncable.Subscription,
) (unsubscriber func(), err error) {
	if sub.Identifier() != c.identifier {
		return nil, errors.Errorf(
			"channel identifier %+v does not match subscription identifier %+v",
			c.identifier, sub.Identifier(),
		)
	}

	cancel, finished := c.subscriber(
		ctx, c.streamName, c.sessionID, func(ctx context.Context, rendered string) (ok bool) {
			return sub.Receive(rendered)
		},
	)
	go func() {
		<-finished
		sub.Close()
	}()
	return cancel, nil
}

func (c *Channel) Perform(data string) error {
	return errors.New("turbo streams channel cannot perform any actions")
}

func NewChannelFactory(
	b *Broker, sessionID string, checkers ...actioncable.IdentifierChecker,
) actioncable.ChannelFactory {
	return func(identifier string) (actioncable.Channel, error) {
		return NewChannel(identifier, b.Hub(), b.Subscribe, sessionID, checkers...)
	}
}
