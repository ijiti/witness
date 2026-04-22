package discovery

import (
	"testing"
	"time"
)

func TestNewBroadcaster(t *testing.T) {
	b := NewBroadcaster()
	if b == nil {
		t.Fatal("NewBroadcaster returned nil")
	}
	if len(b.clients) != 0 {
		t.Fatalf("expected empty clients map, got %d entries", len(b.clients))
	}
}

func TestSubscribeReturnsUniqueIDs(t *testing.T) {
	b := NewBroadcaster()
	id1, ch1 := b.Subscribe()
	id2, ch2 := b.Subscribe()

	if id1 == id2 {
		t.Fatalf("expected unique IDs, got %d and %d", id1, id2)
	}
	if ch1 == nil || ch2 == nil {
		t.Fatal("Subscribe returned nil channel")
	}
}

func TestSendSingleSubscriber(t *testing.T) {
	b := NewBroadcaster()
	_, ch := b.Subscribe()

	ev := WatchEvent{ProjectID: "proj1", Type: "create"}
	b.Send(ev)

	select {
	case got := <-ch:
		if got.ProjectID != ev.ProjectID || got.Type != ev.Type {
			t.Fatalf("expected %+v, got %+v", ev, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSendMultipleSubscribers(t *testing.T) {
	b := NewBroadcaster()
	const n = 5
	channels := make([]<-chan WatchEvent, n)
	for i := range channels {
		_, channels[i] = b.Subscribe()
	}

	ev := WatchEvent{ProjectID: "proj2", Type: "modify"}
	b.Send(ev)

	for i, ch := range channels {
		select {
		case got := <-ch:
			if got.ProjectID != ev.ProjectID || got.Type != ev.Type {
				t.Fatalf("subscriber %d: expected %+v, got %+v", i, ev, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestUnsubscribe(t *testing.T) {
	b := NewBroadcaster()
	id, ch := b.Subscribe()
	b.Unsubscribe(id)

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after Unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out; channel not closed after Unsubscribe")
	}

	// Unsubscribed client should not receive new events.
	ev := WatchEvent{ProjectID: "proj3", Type: "create"}
	b.Send(ev) // should not panic or block
}

func TestSlowClientDrop(t *testing.T) {
	b := NewBroadcaster()
	_, slowCh := b.Subscribe()
	_, fastCh := b.Subscribe()

	// Fill both clients' buffers (capacity 16).
	for i := 0; i < 16; i++ {
		b.Send(WatchEvent{ProjectID: "fill", Type: "create"})
	}

	// Drain the fast client so it has room for the next event.
	for i := 0; i < 16; i++ {
		select {
		case <-fastCh:
		case <-time.After(time.Second):
			t.Fatalf("fast client: timed out draining fill event %d", i)
		}
	}

	// Send one more — should be dropped for slow client, delivered to fast client.
	overflow := WatchEvent{ProjectID: "overflow", Type: "modify"}
	b.Send(overflow)

	// Fast client should receive the overflow event.
	select {
	case got := <-fastCh:
		if got.ProjectID != "overflow" {
			t.Fatalf("fast client: expected overflow event, got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fast client: timed out waiting for overflow event")
	}

	// Slow client's channel should have only the first 16; overflow was dropped.
	drained := 0
	for {
		select {
		case <-slowCh:
			drained++
		default:
			goto done
		}
	}
done:
	if drained != 16 {
		t.Fatalf("slow client: expected 16 buffered events, got %d", drained)
	}
}

func TestSubscribeAfterUnsubscribe(t *testing.T) {
	b := NewBroadcaster()
	id1, _ := b.Subscribe()
	b.Unsubscribe(id1)

	id2, ch2 := b.Subscribe()
	if id2 <= id1 {
		t.Fatalf("expected ID %d > %d (IDs should keep incrementing)", id2, id1)
	}

	// New subscriber should work normally.
	ev := WatchEvent{ProjectID: "proj4", Type: "create"}
	b.Send(ev)
	select {
	case got := <-ch2:
		if got.ProjectID != ev.ProjectID {
			t.Fatalf("expected %+v, got %+v", ev, got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event on re-subscribed channel")
	}
}

func TestDoubleUnsubscribe(t *testing.T) {
	b := NewBroadcaster()
	id, _ := b.Subscribe()
	b.Unsubscribe(id)
	b.Unsubscribe(id) // must not panic
}
