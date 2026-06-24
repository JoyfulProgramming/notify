package evaluations

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"notify/pkg/bus"
)

// TestProperty_FilterIsDeterministic maps to INV-4.
func TestProperty_FilterIsDeterministic(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clearAllRules(t)
		n := generateArbitraryNotification(rt)
		setUserRules(t, generateArbitraryRuleSet(rt))

		id1 := publishNotification(t, n)
		outcome1 := observeRouting(t, id1, 5*time.Second)

		// Same content, new id — same rules must produce the same decision.
		n2 := n
		n2.ID = newUUID(t)
		id2 := publishNotification(t, n2)
		outcome2 := observeRouting(t, id2, 5*time.Second)

		if outcome1 != outcome2 {
			t.Fatalf("same notification routed differently: first=%s second=%s", outcome1, outcome2)
		}
	})
}

// TestProperty_AnyMatchDelivers maps to INV-2, INV-6.
func TestProperty_AnyMatchDelivers(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clearAllRules(t)
		n := generateArbitraryNotification(rt)
		setUserRule(t, generateMatchingRule(rt, n))

		id := publishNotification(t, n)
		assertPresentInStream(t, id, bus.TopicNotificationsMatched, 5*time.Second)
	})
}

// TestProperty_MutuallyExclusiveRouting maps to INV-1, INV-2.
func TestProperty_MutuallyExclusiveRouting(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clearAllRules(t)
		n := generateArbitraryNotification(rt)
		setUserRules(t, generateArbitraryRuleSet(rt))

		id := publishNotification(t, n)
		matched, discarded := waitForRoutingOutcome(t, id, 3*time.Second)

		if matched && discarded {
			t.Fatal("notification appeared in both streams — routing must be mutually exclusive")
		}
		if !matched && !discarded {
			t.Fatal("notification appeared in neither stream — violates INV-1")
		}
	})
}

// TestProperty_DeduplicationByID maps to INV-3.
func TestProperty_DeduplicationByID(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clearAllRules(t)
		n := generateArbitraryDeliverableNotification(rt)
		n.ID = newUUID(t)
		setUserRule(t, generateMatchingRule(rt, n))

		events := subscribeSSE(t)
		defer events.Close() // per-iteration, not t.Cleanup — see sseClient.Close doc

		count := rapid.IntRange(2, 5).Draw(rt, "count")
		for i := 0; i < count; i++ {
			publishNotification(t, n) // same id every time
		}

		// In-process delivery is sub-millisecond; 100ms is generous headroom
		// for collecting any (incorrect) duplicate deliveries without paying
		// a multi-second fixed cost per rapid iteration (this is a fixed
		// collection window, not a wait-for-first-event timeout, so it's
		// always paid in full).
		seen := collectSSEEventsWithID(t, events, n.ID, 100*time.Millisecond)
		if len(seen) != 1 {
			t.Fatalf("expected exactly 1 delivery, got %d", len(seen))
		}
	})
}

// TestProperty_NoSilentDiscard is the system-wide property from plan section 11
// (INV-1, INV-2): a notification with at least one matching rule must never
// be absent from both notifications.matched and notifications.discarded.
func TestProperty_NoSilentDiscard(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clearAllRules(t)
		n := generateArbitraryNotification(rt)
		setUserRule(t, generateMatchingDeliverRule(rt, n))

		id := publishNotification(t, n)

		assertPresentInAtLeastOneStream(t, id,
			[]string{bus.TopicNotificationsMatched, bus.TopicNotificationsDiscarded},
			10*time.Second,
		)
	})
}
