package contracts

import "time"

type RuleChangedKind string

const (
	RuleCreated RuleChangedKind = "CREATED"
	RuleUpdated RuleChangedKind = "UPDATED"
	RuleDeleted RuleChangedKind = "DELETED"
)

// RuleChangedEvent is published to rules.changed topic on any rule mutation.
type RuleChangedEvent struct {
	EventID   string          `json:"event_id"` // UUID
	Kind      RuleChangedKind `json:"kind"`
	Rule      Rule            `json:"rule"`
	ChangedAt time.Time       `json:"changed_at"`
}
