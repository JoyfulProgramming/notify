package evaluations

import (
	"net/http"
	"testing"
	"time"

	"notify/pkg/bus"
	"notify/pkg/contracts"
)

// TestContract_BasicIngestionPublishes maps to INV-1 (plan section 7,
// "Basic ingestion publishes to notifications.captured").
func TestContract_BasicIngestionPublishes(t *testing.T) {
	id := publishViaHTTP(t, contracts.Notification{
		SourceApp: "com.whatsapp",
		Title:     "Alice: hey",
		Body:      "Are you free?",
	})
	n := waitForMessageOnTopic(t, bus.TopicNotificationsCaptured, id, 2*time.Second)
	if n.ID != id {
		t.Fatalf("expected captured message id %s, got %s", id, n.ID)
	}
}

// TestContract_IngestorRejectsMalformed maps to the schema contract: a
// notification missing required fields is rejected with 400.
func TestContract_IngestorRejectsMalformed(t *testing.T) {
	resp := httpPost(t, sys.IngestorURL+"/notifications", "{}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
	}
}

// TestContract_IngestorAssignsReceivedAt maps to the schema contract.
func TestContract_IngestorAssignsReceivedAt(t *testing.T) {
	before := time.Now()
	id := publishViaHTTP(t, contracts.Notification{SourceApp: "com.whatsapp", Title: "Test"})
	msg := waitForMessageOnTopic(t, bus.TopicNotificationsCaptured, id, 3*time.Second)
	after := time.Now()

	if msg.ReceivedAt.Before(before) || msg.ReceivedAt.After(after) {
		t.Fatalf("received_at %v is outside the expected window [%v, %v]", msg.ReceivedAt, before, after)
	}
}

// TestContract_IngestorDeduplicatesByID maps to decision #3: a client-supplied
// id that's POSTed twice must only appear once on notifications.captured.
func TestContract_IngestorDeduplicatesByID(t *testing.T) {
	id := newUUID(t)
	n := contracts.Notification{ID: id, SourceApp: "com.whatsapp", Title: "First"}

	got1 := publishViaHTTP(t, n)
	got2 := publishViaHTTP(t, n)
	if got1 != id || got2 != id {
		t.Fatalf("expected both responses to echo id %s, got %s and %s", id, got1, got2)
	}

	waitForMessageOnTopic(t, bus.TopicNotificationsCaptured, id, 2*time.Second)
	// Give a duplicate publish a moment to land if the dedup were broken.
	time.Sleep(100 * time.Millisecond)
	if count := capturedRecorder.count(id); count != 1 {
		t.Fatalf("expected exactly 1 message on notifications.captured for id %s, got %d", id, count)
	}
}
