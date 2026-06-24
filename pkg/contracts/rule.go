package contracts

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
type Rule struct {
	ID            string // UUID, stable identifier
	UserID        string // owner; "local" for MVP
	SourceApp     string // see matching semantics below
	SourceAccount string // see matching semantics below
	Title         string // see matching semantics below
}

// No Action field in v1. A rule means "surface this notification."
// Matching any rule = deliver. No matching rule = discard. See INV-2, INV-6.

// Matching semantics for SourceApp, SourceAccount, and Title:
//
//	""                 empty   — ignore this field; matches any value
//	"com.google.gmail" exact   — field must equal this value exactly
//	"com.google.*"     pattern — field must match this glob pattern (* = any chars)
//
// Examples:
//
//	SourceApp: ""              matches any app
//	SourceApp: "com.whatsapp"  matches WhatsApp only
//	SourceApp: "com.google.*"  matches any Google app
//	SourceAccount: ""          matches any account
//	SourceAccount: "*@work.com" matches any work email account
//	Title: ""                  matches any title
//	Title: "*invoice*"         matches any title containing "invoice"
//	Title: "Invoice ready"     matches this exact title only
//
// No Priority field. When multiple rules match, the most specific rule wins.
// Specificity: exact > pattern > empty, and more fields populated > fewer. See INV-6.
// Note: SentBy and SentIn matching (contact+group specificity rules) are v2.
// The fields exist on Notification now so the schema does not need versioning when rules are added.
