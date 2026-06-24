package contracts

import "time"

// Notification is the published language crossing all three bounded context boundaries.
// In the Capture context (notifications.captured topic): what an app generated.
// In the Delivery context (notifications.matched topic): what the user will see.
// Schema version: v1. Do not modify fields without versioning.
type Notification struct {
	ID              string            `json:"id"`               // UUID v7, assigned at ingest, immutable, system-wide dedup key
	UserID          string            `json:"user_id"`          // set by ingestor from authenticated token; trusted by all downstream services
	SourceApp       string            `json:"source_app"`       // e.g. "com.whatsapp", "com.google.gmail"
	SourceAccount   string            `json:"source_account"`   // account within the app, e.g. "john@work.com"; empty = unspecified
	SourceID        string            `json:"source_id"`        // item identifier in the source system, e.g. Gmail message ID; empty = not available
	SentBy          string            `json:"sent_by"`          // raw identifier of who sent it: phone, email, username, bot ID; empty = unspecified
	SentIn          string            `json:"sent_in"`          // group/channel/thread context, e.g. WhatsApp group ID; empty = direct/none
	Title           string            `json:"title"`            // may be empty
	Body            string            `json:"body"`             // may be empty
	DeviceID        string            `json:"device_id"`        // UUID identifying the source device
	DeviceTimestamp time.Time         `json:"device_timestamp"` // when the notification appeared on device
	ReceivedAt      time.Time         `json:"received_at"`      // when this system first ingested it
	Metadata        map[string]string `json:"metadata"`         // arbitrary key-value; no service may fail on unknown keys
}
