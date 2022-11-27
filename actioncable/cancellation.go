package actioncable

import (
	"context"
	"sync"
)

// Cancellers associates IDs with context cancellation functions so that those functions can be run
// by ID.
type Cancellers struct {
	funcs map[string][]context.CancelFunc
	m     *sync.Mutex
}

// NewCancellers creates a new instance of Cancellers.
func NewCancellers() *Cancellers {
	return &Cancellers{
		funcs: make(map[string][]context.CancelFunc),
		m:     &sync.Mutex{},
	}
}

// Add records an association between the ID and the context cancellation function.
func (c *Cancellers) Add(id string, canceller context.CancelFunc) {
	// Note: To reduce lock contention at larger scales, we could have a map associating each id to
	// a mutex for the value in the funcs map; but when the map of mutexes is missing a key, we'd need
	// to add an entry. This extra complexity may be justified in the future.
	c.m.Lock()
	defer c.m.Unlock()

	c.funcs[id] = append(c.funcs[id], canceller)
}

// Cancel runs and then forgets all context cancellation functions associated with the ID.
func (c *Cancellers) Cancel(id string) {
	c.m.Lock()
	defer c.m.Unlock()

	for _, canceller := range c.funcs[id] {
		canceller()
	}
	delete(c.funcs, id)
}
