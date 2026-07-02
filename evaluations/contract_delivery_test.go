package evaluations

import (
	"testing"
	"time"
)

// TestContract_FilteredNotificationReachesSSEClient maps to the end-to-end pipeline.
func TestContract_FilteredNotificationReachesSSEClient(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, rawRule{SourceApp: "com.whatsapp"})

	events := subscribeSSE(t)

	id := publishViaHTTP(t, rawNotification{SourceApp: "com.whatsapp", Title: "Hello"})
	got := waitForSSEEventWithID(t, events, id, 5*time.Second)

	if got != id {
		t.Fatalf("expected id %s, got %s", id, got)
	}
}

// TestContract_MultipleClientsEachReceiveNotification — BEHAVIOR: multiple
// connected clients each receive the notification.
func TestContract_MultipleClientsEachReceiveNotification(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, rawRule{SourceApp: "com.whatsapp"})

	clientA := subscribeSSE(t)
	clientB := subscribeSSE(t)

	id := publishViaHTTP(t, rawNotification{SourceApp: "com.whatsapp", Title: "Broadcast"})

	waitForSSEEventWithID(t, clientA, id, 5*time.Second)
	waitForSSEEventWithID(t, clientB, id, 5*time.Second)
}
