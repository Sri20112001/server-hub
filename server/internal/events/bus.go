// Package events is a tiny in-process pub/sub bus with an SSE endpoint.
// It lets the workbench react to deployments, container lifecycle,
// health transitions, gateway reloads, backups and threshold crossings
// without polling.
package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Event is a single broadcast message.
type Event struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp string      `json:"timestamp"`
}

type subscriber struct {
	ch     chan Event
	slow   int
	closed bool
}

// Broker fans events out to SSE subscribers. Slow subscribers miss
// messages (dropped) rather than blocking publishers.
type Broker struct {
	mu     sync.RWMutex
	subs   map[*subscriber]struct{}
	bufLen int
}

func NewBroker() *Broker {
	return &Broker{subs: map[*subscriber]struct{}{}, bufLen: 32}
}

// Publish broadcasts to all current subscribers. Never blocks.
func (b *Broker) Publish(eventType string, data interface{}) {
	ev := Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		if s.closed {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			s.slow++
		}
	}
}

func (b *Broker) subscribe() *subscriber {
	s := &subscriber{ch: make(chan Event, 32)}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

func (b *Broker) unsubscribe(s *subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s.closed = true
	delete(b.subs, s)
	close(s.ch)
}

// Stream serves GET /server-hub/api/events as text/event-stream.
func (b *Broker) Stream(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	_, _ = fmt.Fprintf(c.Writer, ": connected\n\n")
	flusher.Flush()

	sub := b.subscribe()
	defer b.unsubscribe(sub)

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(c.Writer, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-sub.ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", ev.Type, payload)
			flusher.Flush()
		}
	}
}
