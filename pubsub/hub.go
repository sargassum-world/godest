// Package pubsub provides pub-sub functionality.
package pubsub

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

// Receiver

type ReceiveFunc[Message any] func(message Message) (ok bool)

type receiver[Message any] struct {
	topic   string
	receive ReceiveFunc[Message]
	cancel  context.CancelFunc
}

// Hub

type BroadcastingChange struct {
	Added   []string
	Removed []string
}

type Hub[Message any] struct {
	receivers map[string]map[*receiver[Message]]struct{}
	mu        sync.RWMutex
	brChanges chan<- BroadcastingChange
}

func NewHub[Message any](brChanges chan<- BroadcastingChange) *Hub[Message] {
	return &Hub[Message]{
		receivers: make(map[string]map[*receiver[Message]]struct{}),
		brChanges: brChanges,
	}
}

func (h *Hub[Message]) Close() {
	h.mu.Lock() // lock to prevent data race with the Subscribe method sending to brChanges
	defer h.mu.Unlock()

	for _, broadcasting := range h.receivers {
		for receiver := range broadcasting {
			receiver.cancel()
		}
	}

	if h.brChanges != nil {
		close(h.brChanges)
	}
	h.brChanges = nil
}

func (h *Hub[Message]) Subscribe(
	topic string, receive ReceiveFunc[Message],
) (unsubscriber func(), removed <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background()) // FIXME: enable setting parent context?
	r := &receiver[Message]{
		topic:   topic,
		receive: receive,
		cancel:  cancel,
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	broadcasting, ok := h.receivers[topic]
	addedTopic := false
	if !ok {
		broadcasting = make(map[*receiver[Message]]struct{})
		h.receivers[r.topic] = broadcasting
		addedTopic = true
	}
	broadcasting[r] = struct{}{}

	if h.brChanges != nil && addedTopic {
		h.brChanges <- BroadcastingChange{Added: []string{topic}}
	}

	return func() {
		cancel()
		h.unsubscribe(r)
	}, ctx.Done()
}

func (h *Hub[Message]) Cancel(topics ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, topic := range topics {
		broadcasting, ok := h.receivers[topic]
		if !ok {
			continue
		}
		for r := range broadcasting {
			r.cancel()
		}
		delete(h.receivers, topic)
	}

	if h.brChanges != nil && len(topics) > 0 {
		h.brChanges <- BroadcastingChange{Removed: topics}
	}
}

func (h *Hub[Message]) unsubscribe(receivers ...*receiver[Message]) {
	if len(receivers) == 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	removedTopics := make([]string, 0, len(receivers))
	for _, r := range receivers {
		broadcasting, ok := h.receivers[r.topic]
		if !ok {
			continue
		}
		delete(broadcasting, r)
		if len(broadcasting) == 0 {
			delete(h.receivers, r.topic)
			removedTopics = append(removedTopics, r.topic)
		}
		r.cancel()
	}
	if h.brChanges != nil && len(removedTopics) > 0 {
		h.brChanges <- BroadcastingChange{Removed: removedTopics}
	}
}

func (h *Hub[Message]) Broadcast(topic string, message Message) {
	unsubscribe, _ := h.broadcastStrict(topic, message)
	h.unsubscribe(unsubscribe...)
}

func (h *Hub[Message]) broadcastStrict(
	topic string, message Message,
) (unsubscribe []*receiver[Message], err error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	broadcasting, ok := h.receivers[topic]
	if !ok {
		return nil, errors.Errorf("no receivers for %s", topic)
	}
	willUnsubscribe := make(chan *receiver[Message], len(broadcasting))
	wg := sync.WaitGroup{}
	for r := range broadcasting {
		wg.Add(1)
		go func(r *receiver[Message], message Message, willUnsubscribe chan<- *receiver[Message]) {
			defer wg.Done()
			if !r.receive(message) {
				willUnsubscribe <- r
			}
		}(r, message, willUnsubscribe)
	}
	wg.Wait()

	close(willUnsubscribe)
	unsubscribe = make([]*receiver[Message], 0, len(broadcasting))
	for r := range willUnsubscribe {
		unsubscribe = append(unsubscribe, r)
	}
	return unsubscribe, nil
}
