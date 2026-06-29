package evaluations

import (
	"testing"
	"time"

	"notify/pkg/bus"
)

// TestContract_MatchingRuleLeadsToDelivery maps to INV-2.
func TestContract_MatchingRuleLeadsToDelivery(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, ruleWire{SourceApp: "com.whatsapp"})

	id := publishViaHTTP(t, notificationWire{SourceApp: "com.whatsapp", Title: "Alice: hey"})
	assertPresentInStream(t, id, bus.TopicNotificationsMatched, 5*time.Second)
}

// TestContract_NoMatchingRuleRoutesToDiscarded maps to INV-1, INV-2.
func TestContract_NoMatchingRuleRoutesToDiscarded(t *testing.T) {
	clearAllRules(t)

	id := publishViaHTTP(t, notificationWire{SourceApp: "com.twitter", Title: "Someone liked your tweet"})
	assertPresentInStream(t, id, bus.TopicNotificationsDiscarded, 5*time.Second)
	assertAbsentFromStream(t, id, bus.TopicNotificationsMatched, 1*time.Second)
}

// TestContract_NoRuleDefaultsToDiscarded maps to INV-2 (rules are the source of truth).
func TestContract_NoRuleDefaultsToDiscarded(t *testing.T) {
	clearAllRules(t)

	id := publishViaHTTP(t, notificationWire{SourceApp: "com.example.unknown"})
	assertPresentInStream(t, id, bus.TopicNotificationsDiscarded, 5*time.Second)
}

// TestContract_CatchAllRuleMatchesAnyNotification — BEHAVIOR: a rule with all
// fields empty matches any notification.
func TestContract_CatchAllRuleMatchesAnyNotification(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, ruleWire{SourceApp: "*"})

	id := publishViaHTTP(t, notificationWire{SourceApp: "com.example.anything"})
	assertPresentInStream(t, id, bus.TopicNotificationsMatched, 5*time.Second)
}

// TestContract_PatternRuleMatchesGlob — BEHAVIOR: pattern rule matches on glob.
func TestContract_PatternRuleMatchesGlob(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, ruleWire{SourceApp: "com.google.*"})

	matchedID := publishViaHTTP(t, notificationWire{SourceApp: "com.google.gmail"})
	assertPresentInStream(t, matchedID, bus.TopicNotificationsMatched, 5*time.Second)

	discardedID := publishViaHTTP(t, notificationWire{SourceApp: "com.whatsapp"})
	assertPresentInStream(t, discardedID, bus.TopicNotificationsDiscarded, 5*time.Second)
}

// TestContract_TitlePatternNarrowsMatching — BEHAVIOR: title pattern narrows matching.
func TestContract_TitlePatternNarrowsMatching(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, ruleWire{SourceApp: "com.google.gmail", Title: "*invoice*"})

	matchedID := publishViaHTTP(t, notificationWire{
		SourceApp: "com.google.gmail", Title: "Your invoice is ready",
	})
	assertPresentInStream(t, matchedID, bus.TopicNotificationsMatched, 5*time.Second)

	discardedID := publishViaHTTP(t, notificationWire{
		SourceApp: "com.google.gmail", Title: "Newsletter: weekly digest",
	})
	assertPresentInStream(t, discardedID, bus.TopicNotificationsDiscarded, 5*time.Second)
}

// TestContract_RuleChangeAppliesToFutureOnly maps to INV-5.
func TestContract_RuleChangeAppliesToFutureOnly(t *testing.T) {
	clearAllRules(t)

	id1 := publishViaHTTP(t, notificationWire{SourceApp: "com.slack", Title: "Message from Dave"})
	assertPresentInStream(t, id1, bus.TopicNotificationsDiscarded, 5*time.Second)

	setUserRule(t, ruleWire{SourceApp: "com.slack"})

	id2 := publishViaHTTP(t, notificationWire{SourceApp: "com.slack", Title: "Another message"})
	assertPresentInStream(t, id2, bus.TopicNotificationsMatched, 5*time.Second)

	// First notification remains discarded — no retroactive change.
	assertAbsentFromStream(t, id1, bus.TopicNotificationsMatched, 1*time.Second)
}
