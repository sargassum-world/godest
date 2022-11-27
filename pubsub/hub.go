// Package pubsub provides pub-sub functionality. It provides a [Hub] for simple subscription &
// broadcasting management. It also provides a [Broker] for path-based topic routing to pub-sub
// event handlers based on the [github.com/labstack/echo/v4] framework's routing system, as well as
// orchestration of on-demand publishers.
package pubsub

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

// Receiver

// ReceiveFunc is the callback function used to handle each message emitted over a subscription.
type ReceiveFunc[Message any] func(message Message) error

// subscription is the record stored for each active subscription.
type subscription[Message any] struct {
	topic   string
	receive ReceiveFunc[Message]
	cancel  context.CancelFunc
}

// Hub

// BroadcastingChange is an event record listing all topics which were added to the [Hub] and all
// topics which were removed from the [Hub] since the last emitted BroadcastingChange.
type BroadcastingChange struct {
	Added   []string
	Removed []string
}

type broadcasting[Message any] map[*subscription[Message]]struct{}

// Hub coordinates broadcasting of messages between publishers and subscribers.
type Hub[Message any] struct {
	broadcastings map[string]broadcasting[Message]
	mu            sync.RWMutex
	brChanges     chan<- BroadcastingChange
	logger        Logger
}

// NewHub creates a [Hub]. If a send channel of [BroadcastingChange] events is provided, the Hub
// will emit events indicating topics added to the Hub (i.e. a subscription is added on a new topic)
// or removed from the Hub (i.e. all subscriptions on a topic were canceled) as those changes occur.
func NewHub[Message any](brChanges chan<- BroadcastingChange, logger Logger) *Hub[Message] {
	return &Hub[Message]{
		broadcastings: make(map[string]broadcasting[Message]),
		brChanges:     brChanges,
		logger:        logger,
	}
}

// Close cancels all subscriptions and destroys internal resources. If the [Hub] has an associated
// send channel of [BroadcastingChange] events, Close closes and forgets that channel without
// emitting an event listing all removed topics. The Hub should not be used after it's closed.
func (h *Hub[Message]) Close() {
	h.logger.Debug("closing pub-sub hub")
	h.mu.Lock() // lock to prevent data race with the Subscribe method sending to brChanges
	defer h.mu.Unlock()

	h.logger.Debug("canceling subscriptions on pub-sub hub")
	for _, br := range h.broadcastings {
		for subscription := range br {
			subscription.cancel()
		}
	}

	h.logger.Debug("releasing resources on pub-sub hub")
	if h.brChanges != nil {
		close(h.brChanges)
	}
	h.broadcastings = nil
	h.brChanges = nil
}

// Subscribe creates an active subscription on the topic. While the subscription is active, the
// receive callback function will be called on every message published for that topic. The
// subscription is active until the callback function returns an error on a message, the context is
// done, the topic is canceled on the hub, or the hub is closed. The returned channel is closed when
// the subscription becomes inactive.
func (h *Hub[Message]) Subscribe(
	ctx context.Context, topic string, receive ReceiveFunc[Message],
) (removed <-chan struct{}) {
	h.logger.Debugf("adding pub-sub subscription on topic %s", topic)
	cctx, cancel := context.WithCancel(ctx)
	sub := &subscription[Message]{
		topic:   topic,
		receive: receive,
		cancel:  cancel,
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	br, ok := h.broadcastings[topic]
	addedTopic := !ok
	if addedTopic {
		br = make(broadcasting[Message])
		h.broadcastings[sub.topic] = br
	}
	br[sub] = struct{}{}

	if h.brChanges != nil && addedTopic {
		h.brChanges <- BroadcastingChange{Added: []string{topic}}
	}

	go func() {
		<-cctx.Done()
		h.logger.Debugf("removing pub-sub subscription on topic %s", topic)
		h.unsubscribe(sub)
	}()
	return cctx.Done()
}

// Cancel cancels all subscriptions on the topics.
func (h *Hub[Message]) Cancel(topics ...string) {
	h.logger.Debugf("canceling pub-sub subscriptions on topics %v", topics)
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, topic := range topics {
		br, ok := h.broadcastings[topic]
		if !ok {
			continue
		}
		for sub := range br {
			sub.cancel()
		}
		delete(h.broadcastings, topic)
	}

	if h.brChanges != nil && len(topics) > 0 {
		h.brChanges <- BroadcastingChange{Removed: topics}
	}
}

// unsubscribe cancels the subscriptions.
func (h *Hub[Message]) unsubscribe(subscriptions ...*subscription[Message]) {
	if len(subscriptions) == 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	removedTopics := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		br, ok := h.broadcastings[sub.topic]
		if !ok {
			continue
		}
		delete(br, sub)
		if len(br) == 0 {
			delete(h.broadcastings, sub.topic)
			removedTopics = append(removedTopics, sub.topic)
		}
		sub.cancel()
	}
	if h.brChanges != nil && len(removedTopics) > 0 {
		h.brChanges <- BroadcastingChange{Removed: removedTopics}
	}
}

// Broadcast emits a message to all subscriptions on the topic, blocking until completion of
// the receiver callback functions of all subscriptions; those callbacks are run in parallel.
// Subscriptions whose receiver callbacks returned errors are deactivated.
func (h *Hub[Message]) Broadcast(topic string, message Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	br, ok := h.broadcastings[topic]
	if !ok {
		h.logger.Debugf("skipped broadcasting message on %s, since it has no subscriptions", topic)
		return
	}

	willUnsubscribe := make(chan *subscription[Message], len(br))
	wg := sync.WaitGroup{}
	for sub := range br {
		wg.Add(1)
		go func(
			sub *subscription[Message], message Message, willUnsubscribe chan<- *subscription[Message],
		) {
			defer wg.Done()
			if err := sub.receive(message); err != nil {
				h.logger.Warn(errors.Wrapf(
					err, "removing subscription for %s due to message receiver error", topic,
				))
				willUnsubscribe <- sub
			}
		}(sub, message, willUnsubscribe)
	}
	wg.Wait()

	close(willUnsubscribe)
	unsubscribe := make([]*subscription[Message], 0, len(br))
	for sub := range willUnsubscribe {
		unsubscribe = append(unsubscribe, sub)
	}
	h.unsubscribe(unsubscribe...)
}
