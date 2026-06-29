package evaluations

import (
	"pgregory.net/rapid"
)

var sourceApps = []string{"com.whatsapp", "com.google.gmail", "com.slack", "com.instagram", "com.github"}
var titles = []string{"Alice: hey", "Your invoice is ready", "Weekly digest", "New comment on PR #42", ""}

func generateArbitraryNotification(t *rapid.T) notificationWire {
	return notificationWire{
		SourceApp: rapid.SampledFrom(sourceApps).Draw(t, "source_app"),
		Title:     rapid.SampledFrom(titles).Draw(t, "title"),
		Body:      rapid.StringMatching(`[a-zA-Z0-9 ]{0,40}`).Draw(t, "body"),
	}
}

// generateArbitraryDeliverableNotification is the section-10 alias for the
// same notification domain — distinguishes intent at call sites (the
// notification will be paired with a matching rule by the caller).
func generateArbitraryDeliverableNotification(t *rapid.T) notificationWire {
	return generateArbitraryNotification(t)
}

func generateArbitraryRuleSet(t *rapid.T) []ruleWire {
	n := rapid.IntRange(0, 4).Draw(t, "rule_count")
	pool := append([]string{"*"}, sourceApps...)
	rules := make([]ruleWire, 0, n)
	for i := 0; i < n; i++ {
		rules = append(rules, ruleWire{
			SourceApp: rapid.SampledFrom(pool).Draw(t, "rule_source_app"),
		})
	}
	return rules
}

// generateMatchingRule derives a rule guaranteed to match n. Copying
// SourceApp exactly is sufficient: in v1 any matching rule means deliver
// (INV-6), so there's no specificity interaction to construct.
func generateMatchingRule(t *rapid.T, n notificationWire) ruleWire {
	return ruleWire{SourceApp: n.SourceApp}
}

// generateMatchingDeliverRule is the section-11 alias for the same concept.
func generateMatchingDeliverRule(t *rapid.T, n notificationWire) ruleWire {
	return generateMatchingRule(t, n)
}

// generateArbitraryRule generates a rule with arbitrary non-catch-all fields,
// suitable for creation via the HTTP API (which rejects all-fields-empty rules).
// Used by the subset/superset property test (INV-7).
func generateArbitraryRule(t *rapid.T) ruleWire {
	titlePool := []string{"*", "*invoice*", "Weekly digest"}
	return ruleWire{
		SourceApp: rapid.SampledFrom(sourceApps).Draw(t, "new_rule_source_app"),
		Title:     rapid.SampledFrom(titlePool).Draw(t, "new_rule_title"),
	}
}
