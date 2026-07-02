package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Notification is the published language crossing all three bounded context boundaries.
// In the Capture context (notifications.captured topic): what an app generated.
// In the Delivery context (notifications.matched topic): what the user will see.
// Schema version: v1. Do not modify fields without versioning.
//
// Fields are unexported: NewNotification is the only way to obtain a
// Notification (decoding from JSON goes through it too, via UnmarshalJSON),
// so any Notification value in hand is guaranteed to satisfy
// specs/notification.json's required fields.
type Notification struct {
	id              string
	userID          string
	sourceApp       string
	sourceAccount   string
	sourceID        string
	sentBy          string
	sentIn          string
	title           string
	body            string
	deviceID        string
	deviceTimestamp time.Time
	receivedAt      time.Time
	metadata        map[string]string
}

// NotificationParams is the field-by-field input to NewNotification, named
// for call-site readability (a bare positional 12-argument constructor would
// be unreadable).
type NotificationParams struct {
	ID              string
	UserID          string
	SourceApp       string
	SourceAccount   string
	SourceID        string
	SentBy          string
	SentIn          string
	Title           string
	Body            string
	DeviceID        string
	DeviceTimestamp time.Time
	ReceivedAt      time.Time
	Metadata        map[string]string
}

// NewNotification constructs a Notification, or rejects the data outright.
// Per specs/notification.json: id, user_id, source_app, device_timestamp,
// and received_at are required; device_id, if present, must be a UUID.
// Everything else is optional and unconstrained.
func NewNotification(p NotificationParams) (Notification, error) {
	if err := validateNotificationRequired(p); err != nil {
		return Notification{}, err
	}
	if err := validateNotificationUUIDs(p); err != nil {
		return Notification{}, err
	}
	return Notification{
		id:              p.ID,
		userID:          p.UserID,
		sourceApp:       p.SourceApp,
		sourceAccount:   p.SourceAccount,
		sourceID:        p.SourceID,
		sentBy:          p.SentBy,
		sentIn:          p.SentIn,
		title:           p.Title,
		body:            p.Body,
		deviceID:        p.DeviceID,
		deviceTimestamp: p.DeviceTimestamp,
		receivedAt:      p.ReceivedAt,
		metadata:        p.Metadata,
	}, nil
}

func validateNotificationRequired(p NotificationParams) error {
	required := []struct{ name, value string }{
		{"id", p.ID},
		{"user_id", p.UserID},
		{"source_app", p.SourceApp},
	}
	for _, r := range required {
		if r.value == "" {
			return fmt.Errorf("notification: %s is required", r.name)
		}
	}
	if p.DeviceTimestamp.IsZero() {
		return errors.New("notification: device_timestamp is required")
	}
	if p.ReceivedAt.IsZero() {
		return errors.New("notification: received_at is required")
	}
	return nil
}

func validateNotificationUUIDs(p NotificationParams) error {
	if _, err := uuid.Parse(p.ID); err != nil {
		return fmt.Errorf("notification: id must be a valid UUID: %w", err)
	}
	if p.DeviceID != "" {
		if _, err := uuid.Parse(p.DeviceID); err != nil {
			return fmt.Errorf("notification: device_id must be a valid UUID: %w", err)
		}
	}
	return nil
}

func (n Notification) ID() string                  { return n.id }
func (n Notification) UserID() string              { return n.userID }
func (n Notification) SourceApp() string           { return n.sourceApp }
func (n Notification) SourceAccount() string       { return n.sourceAccount }
func (n Notification) SourceID() string            { return n.sourceID }
func (n Notification) SentBy() string              { return n.sentBy }
func (n Notification) SentIn() string              { return n.sentIn }
func (n Notification) Title() string               { return n.title }
func (n Notification) Body() string                { return n.body }
func (n Notification) DeviceID() string            { return n.deviceID }
func (n Notification) DeviceTimestamp() time.Time  { return n.deviceTimestamp }
func (n Notification) ReceivedAt() time.Time       { return n.receivedAt }
func (n Notification) Metadata() map[string]string { return n.metadata }

// notificationWire mirrors the JSON wire shape (specs/notification.json
// json_key names). MarshalJSON/UnmarshalJSON go through it so a
// Notification still round-trips across the bus and SSE without exposing
// its fields directly — decoding invalid/incomplete JSON fails outright
// rather than producing a half-valid Notification.
type notificationWire struct {
	ID              string            `json:"id"`
	UserID          string            `json:"user_id"`
	SourceApp       string            `json:"source_app"`
	SourceAccount   string            `json:"source_account"`
	SourceID        string            `json:"source_id"`
	SentBy          string            `json:"sent_by"`
	SentIn          string            `json:"sent_in"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	DeviceID        string            `json:"device_id"`
	DeviceTimestamp time.Time         `json:"device_timestamp"`
	ReceivedAt      time.Time         `json:"received_at"`
	Metadata        map[string]string `json:"metadata"`
}

func (n Notification) MarshalJSON() ([]byte, error) {
	return json.Marshal(notificationWire{
		ID:              n.id,
		UserID:          n.userID,
		SourceApp:       n.sourceApp,
		SourceAccount:   n.sourceAccount,
		SourceID:        n.sourceID,
		SentBy:          n.sentBy,
		SentIn:          n.sentIn,
		Title:           n.title,
		Body:            n.body,
		DeviceID:        n.deviceID,
		DeviceTimestamp: n.deviceTimestamp,
		ReceivedAt:      n.receivedAt,
		Metadata:        n.metadata,
	})
}

func (n *Notification) UnmarshalJSON(data []byte) error {
	var w notificationWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	parsed, err := NewNotification(NotificationParams{
		ID:              w.ID,
		UserID:          w.UserID,
		SourceApp:       w.SourceApp,
		SourceAccount:   w.SourceAccount,
		SourceID:        w.SourceID,
		SentBy:          w.SentBy,
		SentIn:          w.SentIn,
		Title:           w.Title,
		Body:            w.Body,
		DeviceID:        w.DeviceID,
		DeviceTimestamp: w.DeviceTimestamp,
		ReceivedAt:      w.ReceivedAt,
		Metadata:        w.Metadata,
	})
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}
