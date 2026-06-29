package evaluations

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"notify/pkg/bus"
	"notify/pkg/contracts"
	"notify/pkg/testharness"
)

// notificationRecorder gives stream assertions a way to look back in time:
// it subscribes to a topic for the lifetime of the test run and remembers
// every notification id it has seen, so assertPresentInStream can succeed
// even when called after the publish that produced the message (the
// in-memory bus, like a real Pub/Sub subscription, only delivers messages
// published after the subscription existed — so the subscription has to be
// long-lived, not created on demand inside each assertion).
type notificationRecorder struct {
	mu     sync.Mutex
	seen   map[string]contracts.Notification
	counts map[string]int
}

func newNotificationRecorder(b bus.Bus, topic string) *notificationRecorder {
	rec := &notificationRecorder{
		seen:   make(map[string]contracts.Notification),
		counts: make(map[string]int),
	}
	sub := b.Subscribe(topic, nil)
	go func() {
		for {
			msg, ack, _, err := sub.Receive(context.Background())
			if err != nil {
				return
			}
			var n contracts.Notification
			if err := json.Unmarshal(msg.Data, &n); err == nil {
				rec.mu.Lock()
				rec.seen[n.ID()] = n
				rec.counts[n.ID()]++
				rec.mu.Unlock()
			}
			ack()
		}
	}()
	return rec
}

// count returns how many times a message with this id has appeared on the
// topic — used to assert exact-once delivery (e.g. ingestor dedup).
func (r *notificationRecorder) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[id]
}

func (r *notificationRecorder) has(id string) bool {
	_, ok := r.get(id)
	return ok
}

func (r *notificationRecorder) get(id string) (contracts.Notification, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.seen[id]
	return n, ok
}

func (r *notificationRecorder) waitFor(id string, timeout time.Duration) (contracts.Notification, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if n, ok := r.get(id); ok {
			return n, true
		}
		if time.Now().After(deadline) {
			return contracts.Notification{}, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ruleEventRecorder is the same idea for the rules.changed topic.
type ruleEventRecorder struct {
	mu     sync.Mutex
	events map[string]map[contracts.RuleChangedKind]bool
}

func newRuleEventRecorder(b bus.Bus) *ruleEventRecorder {
	rec := &ruleEventRecorder{events: make(map[string]map[contracts.RuleChangedKind]bool)}
	sub := b.Subscribe(bus.TopicRulesChanged, nil)
	go func() {
		for {
			msg, ack, _, err := sub.Receive(context.Background())
			if err != nil {
				return
			}
			var ev contracts.RuleChangedEvent
			if err := json.Unmarshal(msg.Data, &ev); err == nil {
				rec.mu.Lock()
				if rec.events[ev.Rule().ID()] == nil {
					rec.events[ev.Rule().ID()] = make(map[contracts.RuleChangedKind]bool)
				}
				rec.events[ev.Rule().ID()][ev.Kind()] = true
				rec.mu.Unlock()
			}
			ack()
		}
	}()
	return rec
}

func (r *ruleEventRecorder) has(ruleID string, kind contracts.RuleChangedKind) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events[ruleID][kind]
}

var (
	capturedRecorder   *notificationRecorder
	matchedRecorder    *notificationRecorder
	discardedRecorder  *notificationRecorder
	ruleEventsRecorder *ruleEventRecorder
)

func startRecorders(sys *testharness.System) {
	capturedRecorder = newNotificationRecorder(sys.Bus, bus.TopicNotificationsCaptured)
	matchedRecorder = newNotificationRecorder(sys.Bus, bus.TopicNotificationsMatched)
	discardedRecorder = newNotificationRecorder(sys.Bus, bus.TopicNotificationsDiscarded)
	ruleEventsRecorder = newRuleEventRecorder(sys.Bus)
}

func recorderFor(topic string) *notificationRecorder {
	switch topic {
	case bus.TopicNotificationsCaptured:
		return capturedRecorder
	case bus.TopicNotificationsMatched:
		return matchedRecorder
	case bus.TopicNotificationsDiscarded:
		return discardedRecorder
	default:
		panic("evaluations: no recorder for topic " + topic)
	}
}
