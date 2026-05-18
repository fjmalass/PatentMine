package engine

import (
	"sync"

	"patentmine/internal/proto"
)

// subBuffer is the per-subscriber event queue depth. A subscriber that falls
// behind loses its oldest events rather than stalling the publisher.
const subBuffer = 64

// Bus is a fan-out broadcaster for daemon->client events. Publishing never
// blocks: a slow subscriber drops its oldest event so it cannot stall the
// engine or other subscribers.
type Bus struct {
	mu     sync.Mutex
	subs   map[int]chan proto.Event
	nextID int
	closed bool
}

// NewBus creates an empty Bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[int]chan proto.Event)}
}

// Subscribe registers a subscriber and returns its event channel plus a
// cancel function that unsubscribes and closes the channel.
func (b *Bus) Subscribe() (<-chan proto.Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan proto.Event, subBuffer)
	if b.closed {
		close(ch)
		return ch, func() {}
	}
	id := b.nextID
	b.nextID++
	b.subs[id] = ch

	return ch, func() { b.unsubscribe(id) }
}

func (b *Bus) unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(ch)
	}
}

// Publish delivers ev to every subscriber without blocking. When a subscriber
// buffer is full its oldest event is discarded to make room.
func (b *Bus) Publish(ev proto.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs {
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

// Close shuts the Bus down and closes every subscriber channel.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, ch := range b.subs {
		delete(b.subs, id)
		close(ch)
	}
}
