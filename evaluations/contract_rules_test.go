package evaluations

import (
	"net/http"
	"testing"
	"time"

	"notify/pkg/contracts"
)

// TestContract_CreateRuleEmitsEvent maps to INV-5.
func TestContract_CreateRuleEmitsEvent(t *testing.T) {
	id := createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.whatsapp"})
	assertRuleExistsViaHTTP(t, id)
	waitForRuleEvent(t, id, contracts.RuleCreated, 3*time.Second)
}

// TestContract_DeleteRuleEmitsEvent maps to INV-5.
func TestContract_DeleteRuleEmitsEvent(t *testing.T) {
	id := createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.whatsapp"})
	deleteRuleViaHTTP(t, id)
	assertRuleAbsentViaHTTP(t, id)
	waitForRuleEvent(t, id, contracts.RuleDeleted, 3*time.Second)
}

// TestContract_CreateRuleWithMissingFieldRejected — BEHAVIOR: an empty body
// (no fields at all) is rejected.
func TestContract_CreateRuleWithMissingFieldRejected(t *testing.T) {
	resp := httpPost(t, sys.RulesURL+"/rules", "{}")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty rule body, got %d", resp.StatusCode)
	}
}

// TestContract_AddingPatternRuleRemovesExactSubset maps to INV-7: a more
// generic rule (pattern) supersedes exact rules it covers.
func TestContract_AddingPatternRuleRemovesExactSubset(t *testing.T) {
	clearAllRules(t)
	exactID := createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.google.gmail"})
	assertRuleExistsViaHTTP(t, exactID)

	createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.google.*"})

	assertRuleAbsentViaHTTP(t, exactID)
}

// TestContract_AddingExactRuleRemovesMatchingPattern maps to INV-7: a more
// specific rule (exact) displaces any pattern rule it falls within.
func TestContract_AddingExactRuleRemovesMatchingPattern(t *testing.T) {
	clearAllRules(t)
	patternID := createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.google.*"})
	assertRuleExistsViaHTTP(t, patternID)

	createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.google.gmail"})

	assertRuleAbsentViaHTTP(t, patternID)
}

// TestContract_AddingSpecificRuleRemovesGenericCatchAll maps to INV-7: adding
// any specific rule displaces a pre-existing catch-all (all-fields-empty) rule.
// The catch-all is set up via setUserRule because the HTTP API intentionally
// rejects all-fields-empty rules as too ambiguous to create explicitly.
func TestContract_AddingSpecificRuleRemovesGenericCatchAll(t *testing.T) {
	clearAllRules(t)
	catchAllID := newUUID(t)
	setUserRule(t, contracts.Rule{ID: catchAllID, SourceApp: ""})
	assertRuleExistsViaHTTP(t, catchAllID)

	createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.whatsapp"})

	assertRuleAbsentViaHTTP(t, catchAllID)
}

// TestContract_UnrelatedRulesArePreserved maps to INV-7: rules with no
// subset-or-superset relationship coexist without affecting each other.
func TestContract_UnrelatedRulesArePreserved(t *testing.T) {
	clearAllRules(t)
	whatsappID := createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.whatsapp"})
	slackID := createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.slack"})

	createRuleViaHTTP(t, contracts.Rule{SourceApp: "com.github"})

	assertRuleExistsViaHTTP(t, whatsappID)
	assertRuleExistsViaHTTP(t, slackID)
}
