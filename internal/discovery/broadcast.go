package discovery

import (
	"sync"
	"sync/atomic"
)

// Broadcaster fans out watch events to multiple SSE clients.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[uint64]chan WatchEvent
	nextID  atomic.Uint64
}

// NewBroadcaster creates a new event broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[uint64]chan WatchEvent),
	}
}

// Subscribe registers a new client and returns its event channel and ID.
func (b *Broadcaster) Subscribe() (uint64, <-chan WatchEvent) {
	id := b.nextID.Add(1)
	ch := make(chan WatchEvent, 16)
	b.mu.Lock()
	b.clients[id] = ch
	b.mu.Unlock()
	return id, ch
}

// Unsubscribe removes a client and closes its channel.
func (b *Broadcaster) Unsubscribe(id uint64) {
	b.mu.Lock()
	if ch, ok := b.clients[id]; ok {
		delete(b.clients, id)
		close(ch)
	}
	b.mu.Unlock()
}

// Send broadcasts an event to all subscribed clients.
func (b *Broadcaster) Send(ev WatchEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.clients {
		select {
		case ch <- ev:
		default:
			// Channel full — client is slow, drop event.
		}
	}
}

