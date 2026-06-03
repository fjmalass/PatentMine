package engine

import (
	"testing"

	"patentmine/internal/proto"
)

func dbEvent() proto.Event {
	return proto.NewEvent(proto.EventDBChanged, proto.Empty{})
}

func TestBusFanOut(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	ch1, unsub1 := bus.Subscribe()
	ch2, _ := bus.Subscribe()

	bus.Publish(dbEvent())
	if (<-ch1).Method != proto.EventDBChanged {
		t.Fatal("subscriber 1 missed the event")
	}
	if (<-ch2).Method != proto.EventDBChanged {
		t.Fatal("subscriber 2 missed the event")
	}

	unsub1()
	if _, ok := <-ch1; ok {
		t.Fatal("unsubscribed channel should be closed")
	}

	// Publishing after unsubscribe still reaches the remaining subscriber.
	bus.Publish(dbEvent())
	if (<-ch2).Method != proto.EventDBChanged {
		t.Fatal("subscriber 2 missed event after subscriber 1 left")
	}
}

func TestBusDropsOldestWhenSubscriberLags(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	ch, _ := bus.Subscribe()
	// Publish more than the buffer holds; Publish must never block.
	for range subBuffer + 50 {
		bus.Publish(dbEvent())
	}
	if len(ch) != subBuffer {
		t.Fatalf("buffered %d events, want %d (drop-oldest)", len(ch), subBuffer)
	}
}

func TestBusCloseClosesSubscribers(t *testing.T) {
	bus := NewBus()
	ch, _ := bus.Subscribe()
	bus.Close()
	if _, ok := <-ch; ok {
		t.Fatal("Close should close subscriber channels")
	}
	// Publish and Close after Close are safe no-ops.
	bus.Publish(dbEvent())
	bus.Close()
}

func TestBusPublishConcurrentlyClosed(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			_, unsub := bus.Subscribe()
			unsub()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			bus.Publish(dbEvent())
		}
		done <- true
	}()

	<-done
	<-done
}

