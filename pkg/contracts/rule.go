package contracts

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Action is reserved for v2, when exception rules (DISCARD) are introduced —
// e.g. "everyone in this group except Pete". In v1, a rule means "surface this."
// Matching any rule = deliver. No match = discard. No explicit action needed.
type Action string

const (
	ActionDeliver Action = "DELIVER"
	ActionDiscard Action = "DISCARD"
)

// Rule is a pure domain type. It carries no JSON tags — serialisation is an
// adapter concern handled at each service boundary (rule-api HTTP handler,
// RuleChangedEvent publisher). Only Notification and RuleChangedEvent are
// wire formats and carry JSON tags.
//
// Fields are unexported: NewRule is the only way to obtain a Rule, so any
// Rule value in hand is guaranteed to satisfy specs/rule.json's required
// fields — there is no "construct now, validate later" path.
type Rule struct {
	id            string // UUID, stable identifier
	userID        string // owner; "local" for MVP
	sourceApp     string // see matching semantics below
	sourceAccount string // see matching semantics below
	title         string // see matching semantics below
}

// RuleParams holds the fields needed to construct a Rule via NewRule.
type RuleParams struct {
	ID            string
	UserID        string
	SourceApp     string
	SourceAccount string
	Title         string
}

// NewRule constructs a Rule, or rejects the data outright. ID and UserID are
// required by specs/rule.json; ID must additionally parse as a UUID.
// SourceApp/SourceAccount/Title are optional match patterns (see matching
// semantics below) and accept any string, including "".
func NewRule(p RuleParams) (Rule, error) {
	if p.ID == "" {
		return Rule{}, errors.New("rule: id is required")
	}
	if _, err := uuid.Parse(p.ID); err != nil {
		return Rule{}, fmt.Errorf("rule: id must be a valid UUID: %w", err)
	}
	if p.UserID == "" {
		return Rule{}, errors.New("rule: user_id is required")
	}
	return Rule{
		id:            p.ID,
		userID:        p.UserID,
		sourceApp:     p.SourceApp,
		sourceAccount: p.SourceAccount,
		title:         p.Title,
	}, nil
}

func (r Rule) ID() string            { return r.id }
func (r Rule) UserID() string        { return r.userID }
func (r Rule) SourceApp() string     { return r.sourceApp }
func (r Rule) SourceAccount() string { return r.sourceAccount }
func (r Rule) Title() string         { return r.title }

// No Action field in v1. A rule means "surface this notification."
// Matching any rule = deliver. No matching rule = discard. See INV-2, INV-6.

// Matching semantics for SourceApp, SourceAccount, and Title:
//
//	"*"                wildcard — matches any value for this field
//	"com.google.gmail" exact    — field must equal this value exactly
//	"com.google.*"     pattern  — field must match this glob pattern (* = any chars)
//
// Examples:
//
//	SourceApp: "*"              matches any app
//	SourceApp: "com.whatsapp"  matches WhatsApp only
//	SourceApp: "com.google.*"  matches any Google app
//	SourceAccount: "*"          matches any account
//	SourceAccount: "*@work.com" matches any work email account
//	Title: "*"                 matches any title
//	Title: "*invoice*"         matches any title containing "invoice"
//	Title: "Invoice ready"     matches this exact title only
//
// No Priority field. When multiple rules match, the most specific rule wins.
// Specificity: exact > pattern > empty, and more fields populated > fewer. See INV-6.
// Note: SentBy and SentIn matching (contact+group specificity rules) are v2.
// The fields exist on Notification now so the schema does not need versioning when rules are added.
