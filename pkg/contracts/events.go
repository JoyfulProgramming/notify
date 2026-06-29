package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type RuleChangedKind string

const (
	RuleCreated RuleChangedKind = "CREATED"
	RuleUpdated RuleChangedKind = "UPDATED"
	RuleDeleted RuleChangedKind = "DELETED"
)

func (k RuleChangedKind) valid() bool {
	switch k {
	case RuleCreated, RuleUpdated, RuleDeleted:
		return true
	default:
		return false
	}
}

// RuleChangedEvent is published to rules.changed topic on any rule mutation.
//
// Fields are unexported: NewRuleChangedEvent is the only way to obtain one
// (decoding from JSON goes through it too, via UnmarshalJSON), so any
// RuleChangedEvent value in hand is guaranteed to satisfy
// specs/rule_changed_event.json's required fields.
type RuleChangedEvent struct {
	eventID   string
	kind      RuleChangedKind
	rule      Rule
	changedAt time.Time
}

// NewRuleChangedEvent constructs a RuleChangedEvent, or rejects the data
// outright. Per specs/rule_changed_event.json: event_id, kind, rule, and
// changed_at are all required; event_id must be a UUID and kind must be one
// of CREATED/UPDATED/DELETED. rule is a Rule, so it's already valid by
// construction — no separate check needed.
func NewRuleChangedEvent(eventID string, kind RuleChangedKind, rule Rule, changedAt time.Time) (RuleChangedEvent, error) {
	if eventID == "" {
		return RuleChangedEvent{}, errors.New("rule_changed_event: event_id is required")
	}
	if _, err := uuid.Parse(eventID); err != nil {
		return RuleChangedEvent{}, fmt.Errorf("rule_changed_event: event_id must be a valid UUID: %w", err)
	}
	if !kind.valid() {
		return RuleChangedEvent{}, fmt.Errorf("rule_changed_event: kind must be one of CREATED/UPDATED/DELETED, got %q", kind)
	}
	if changedAt.IsZero() {
		return RuleChangedEvent{}, errors.New("rule_changed_event: changed_at is required")
	}
	return RuleChangedEvent{eventID: eventID, kind: kind, rule: rule, changedAt: changedAt}, nil
}

func (e RuleChangedEvent) EventID() string       { return e.eventID }
func (e RuleChangedEvent) Kind() RuleChangedKind { return e.kind }
func (e RuleChangedEvent) Rule() Rule            { return e.rule }
func (e RuleChangedEvent) ChangedAt() time.Time  { return e.changedAt }

// ruleWire mirrors a Rule's wire shape for embedding inside
// RuleChangedEvent's JSON — Rule itself carries no JSON tags (see rule.go).
type ruleWire struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	SourceApp     string `json:"source_app"`
	SourceAccount string `json:"source_account"`
	Title         string `json:"title"`
}

type ruleChangedEventWire struct {
	EventID   string          `json:"event_id"`
	Kind      RuleChangedKind `json:"kind"`
	Rule      ruleWire        `json:"rule"`
	ChangedAt time.Time       `json:"changed_at"`
}

func (e RuleChangedEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(ruleChangedEventWire{
		EventID: e.eventID,
		Kind:    e.kind,
		Rule: ruleWire{
			ID:            e.rule.ID(),
			UserID:        e.rule.UserID(),
			SourceApp:     e.rule.SourceApp(),
			SourceAccount: e.rule.SourceAccount(),
			Title:         e.rule.Title(),
		},
		ChangedAt: e.changedAt,
	})
}

func (e *RuleChangedEvent) UnmarshalJSON(data []byte) error {
	var w ruleChangedEventWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	rule, err := NewRule(w.Rule.ID, w.Rule.UserID, w.Rule.SourceApp, w.Rule.SourceAccount, w.Rule.Title)
	if err != nil {
		return err
	}
	parsed, err := NewRuleChangedEvent(w.EventID, w.Kind, rule, w.ChangedAt)
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}
