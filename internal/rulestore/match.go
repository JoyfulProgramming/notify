package rulestore

import (
	"path"

	"notify/pkg/contracts"
)

// matchField implements the per-field semantics from pkg/contracts/rule.go:
// empty pattern matches anything; otherwise an exact match or a glob match
// ("*" = any chars — safe here since none of these fields ever contain "/",
// the only character path.Match treats specially).
func matchField(pattern, value string) bool {
	if pattern == "" {
		return true
	}
	ok, err := path.Match(pattern, value)
	return err == nil && ok
}

// Matches reports whether a single rule matches a notification.
func Matches(r contracts.Rule, n contracts.Notification) bool {
	return matchField(r.SourceApp, n.SourceApp) &&
		matchField(r.SourceAccount, n.SourceAccount) &&
		matchField(r.Title, n.Title)
}

// AnyRuleMatches implements INV-6: in v1 every rule means DELIVER, so matching
// any single rule is sufficient — no specificity resolution is needed.
func AnyRuleMatches(rules []contracts.Rule, n contracts.Notification) bool {
	for _, r := range rules {
		if Matches(r, n) {
			return true
		}
	}
	return false
}
