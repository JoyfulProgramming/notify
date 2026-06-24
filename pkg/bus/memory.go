package bus

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed is returned by Receive once a Subscription has been closed and
// its queue drained.
var ErrClosed = errors.New("bus: subscription closed")

// Memory is an in-process Bus: no Docker, no emulator, no network. Each
// Subscribe call gets its own ordered, unbounded queue fed by Publish — the
// same fan-out semantics as one Pub/Sub subscription per consumer.
type Memory struct {
	mu     sync.Mutex
	topics map[string][]*Subscription
}

func NewMemory() *Memory {
	return &Memory{topics: make(map[string][]*Subscription)}
}

func (b *Memory) Publish(topic string, msg Message) error {
	b.mu.Lock()
	subs := append([]*Subscription(nil), b.topics[topic]...)
	b.mu.Unlock()

	for _, s := range subs {
		if s.filter == nil || s.filter(msg) {
			s.enqueue(msg)
		}
	}
	return nil
}

func (b *Memory) Subscribe(topic string, filter Filter) *Subscription {
	s := &Subscription{
		topic:  topic,
		filter: filter,
		bus:    b,
		notify: make(chan struct{}, 1),
	}
	b.mu.Lock()
	b.topics[topic] = append(b.topics[topic], s)
	b.mu.Unlock()
	return s
}

func (b *Memory) unsubscribe(s *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.topics[s.topic]
	for i, x := range subs {
		if x == s {
			b.topics[s.topic] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// Subscription is one consumer's ordered view of a topic. Nack re-enqueues
// the message on this same subscription for redelivery — the local
// equivalent of Pub/Sub redelivering a nacked message (see plan decision #4:
// infra failure must never be confused with a discard decision).
type Subscription struct {
	topic  string
	filter Filter
	bus    *Memory

	mu     sync.Mutex
	queue  []Message
	notify chan struct{}
	closed bool
}

func (s *Subscription) enqueue(msg Message) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.queue = append(s.queue, msg)
	s.mu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Receive blocks until a message is available, ctx is done, or the
// subscription is closed and drained.
func (s *Subscription) Receive(ctx context.Context) (Message, AckFunc, NackFunc, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			msg := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()

			ack := func() {}
			nack := func() { s.enqueue(msg) }
			return msg, ack, nack, nil
		}
		closed := s.closed
		s.mu.Unlock()

		if closed {
			return Message{}, nil, nil, ErrClosed
		}

		select {
		case <-s.notify:
			continue
		case <-ctx.Done():
			return Message{}, nil, nil, ctx.Err()
		}
	}
}

// Close detaches the subscription from its topic. Any goroutine blocked in
// Receive wakes up and observes ErrClosed once the queue is drained.
func (s *Subscription) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	select {
	case s.notify <- struct{}{}:
	default:
	}

	s.bus.unsubscribe(s)
}
