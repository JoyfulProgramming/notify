package evaluations

import (
	"pgregory.net/rapid"

	"notify/pkg/contracts"
)

var sourceApps = []string{"com.whatsapp", "com.google.gmail", "com.slack", "com.instagram", "com.github"}
var titles = []string{"Alice: hey", "Your invoice is ready", "Weekly digest", "New comment on PR #42", ""}

func generateArbitraryNotification(t *rapid.T) contracts.Notification {
	return contracts.Notification{
		SourceApp: rapid.SampledFrom(sourceApps).Draw(t, "source_app"),
		Title:     rapid.SampledFrom(titles).Draw(t, "title"),
		Body:      rapid.StringMatching(`[a-zA-Z0-9 ]{0,40}`).Draw(t, "body"),
	}
}

// generateArbitraryDeliverableNotification is the section-10 alias for the
// same notification domain — distinguishes intent at call sites (the
// notification will be paired with a matching rule by the caller).
func generateArbitraryDeliverableNotification(t *rapid.T) contracts.Notification {
	return generateArbitraryNotification(t)
}

func generateArbitraryRuleSet(t *rapid.T) []contracts.Rule {
	n := rapid.IntRange(0, 4).Draw(t, "rule_count")
	pool := append([]string{""}, sourceApps...)
	rules := make([]contracts.Rule, 0, n)
	for i := 0; i < n; i++ {
		rules = append(rules, contracts.Rule{
			SourceApp: rapid.SampledFrom(pool).Draw(t, "rule_source_app"),
		})
	}
	return rules
}

// generateMatchingRule derives a rule guaranteed to match n. Copying
// SourceApp exactly is sufficient: in v1 any matching rule means deliver
// (INV-6), so there's no specificity interaction to construct.
func generateMatchingRule(t *rapid.T, n contracts.Notification) contracts.Rule {
	return contracts.Rule{SourceApp: n.SourceApp}
}

// generateMatchingDeliverRule is the section-11 alias for the same concept.
func generateMatchingDeliverRule(t *rapid.T, n contracts.Notification) contracts.Rule {
	return generateMatchingRule(t, n)
}
