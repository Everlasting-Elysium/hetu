package kernel

import (
	"context"
	"sync"
)

// Event types published on the bus. Plugins subscribe to react to indexing.
const (
	EventAssetIndexed = "asset.indexed"
	EventScanFinished = "scan.finished"
)

// Event is a message on the in-process event bus.
type Event struct {
	Type string
	Data any
}

// EventHandler consumes an event.
type EventHandler func(ctx context.Context, e Event)

// EventBus is a minimal in-process publish/subscribe bus. It is synchronous:
// handlers run in the publisher's goroutine.
type EventBus struct {
	mu   sync.RWMutex
	subs map[string][]EventHandler
}

// NewEventBus returns an empty bus.
func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[string][]EventHandler)}
}

// Subscribe registers h for the given event type.
func (b *EventBus) Subscribe(eventType string, h EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[eventType] = append(b.subs[eventType], h)
}

// Publish delivers e to all handlers subscribed to e.Type.
func (b *EventBus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	handlers := b.subs[e.Type]
	b.mu.RUnlock()
	for _, h := range handlers {
		h(ctx, e)
	}
}
