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
