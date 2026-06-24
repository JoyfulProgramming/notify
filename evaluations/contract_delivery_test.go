package evaluations

import (
	"testing"
	"time"

	"notify/pkg/contracts"
)

// TestContract_FilteredNotificationReachesSSEClient maps to the end-to-end pipeline.
func TestContract_FilteredNotificationReachesSSEClient(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, contracts.Rule{SourceApp: "com.whatsapp"})

	events := subscribeSSE(t)

	id := publishViaHTTP(t, contracts.Notification{SourceApp: "com.whatsapp", Title: "Hello"})
	event := waitForSSEEventWithID(t, events, id, 5*time.Second)

	if event.ID != id {
		t.Fatalf("expected id %s, got %s", id, event.ID)
	}
}

// TestContract_MultipleClientsEachReceiveNotification — BEHAVIOR: multiple
// connected clients each receive the notification.
func TestContract_MultipleClientsEachReceiveNotification(t *testing.T) {
	clearAllRules(t)
	setUserRule(t, contracts.Rule{SourceApp: "com.whatsapp"})

	clientA := subscribeSSE(t)
	clientB := subscribeSSE(t)

	id := publishViaHTTP(t, contracts.Notification{SourceApp: "com.whatsapp", Title: "Broadcast"})

	waitForSSEEventWithID(t, clientA, id, 5*time.Second)
	waitForSSEEventWithID(t, clientB, id, 5*time.Second)
}
