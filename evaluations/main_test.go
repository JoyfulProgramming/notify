// Package evaluations contains only durable evaluations — tests that speak
// exclusively in public contracts (plan section 4). They import
// notify/pkg/contracts and notify/pkg/testharness, never notify/cmd or
// notify/internal directly.
package evaluations

import (
	"os"
	"testing"

	"notify/pkg/testharness"
)

var sys *testharness.System

// TestMain boots one shared System for the whole package: the plan's
// contract/property test pseudocode assumes persistent, mutable rule state
// across test functions (setUserRule/clearAllRules between cases), so tests
// in this package must run sequentially, not in parallel.
func TestMain(m *testing.M) {
	var err error
	sys, err = testharness.StartForSuite()
	if err != nil {
		panic(err)
	}

	startRecorders(sys)

	code := m.Run()
	sys.Close()
	os.Exit(code)
}
