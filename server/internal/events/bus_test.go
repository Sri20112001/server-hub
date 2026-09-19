package events

import (
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	b := NewBroker()
	sub := b.subscribe()
	defer b.unsubscribe(sub)

	b.Publish("deployment.completed", map[string]interface{}{"project": "x"})
	select {
	case ev := <-sub.ch:
		if ev.Type != "deployment.completed" || ev.Timestamp == "" {
			t.Fatalf("bad event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event received")
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	b := NewBroker()
	sub := b.subscribe()
	defer b.unsubscribe(sub)
	// Fill the buffer and keep publishing — must return immediately.
	for i := 0; i < 100; i++ {
		done := make(chan struct{})
		go func() {
			b.Publish("test", i)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("publish blocked on slow subscriber")
		}
	}
}
