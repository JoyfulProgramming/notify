// Package testharness boots the full Notify pipeline in-process for the
// evaluations suite. It is the one package allowed to import internal/* on
// behalf of evaluations — see notify_mvp_plan.md section 4 and the plan-mode
// deviation note in /Users/johngallagher/.claude/plans/partitioned-plotting-cloud.md:
// evaluations themselves only ever see this package's public surface
// (URLs + a bus handle), never the service internals directly.
package testharness

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"notify/internal/deliver"
	"notify/internal/filter"
	"notify/internal/ingestor"
	"notify/internal/rules"
	"notify/internal/rulestore"
	"notify/pkg/bus"
)

// System is a fresh, fully wired instance of the pipeline: ingestor, filter,
// rule-api, and delivery-service sharing one in-memory bus and one SQLite
// rule store, equivalent to one cmd/notify-local process.
type System struct {
	Bus   bus.Bus
	Store *rulestore.Store

	IngestorURL string
	RulesURL    string
	DeliverURL  string

	teardown func()
}

// Close releases everything started by Start/StartForSuite. Start already
// registers this via t.Cleanup; only StartForSuite callers need to call it
// explicitly (typically from TestMain).
func (s *System) Close() {
	s.teardown()
}

// Start boots a System for the duration of the test and registers teardown
// via t.Cleanup.
func Start(t testing.TB) *System {
	t.Helper()
	sys, err := boot(t.TempDir())
	if err != nil {
		t.Fatalf("testharness: %v", err)
	}
	t.Cleanup(sys.Close)
	return sys
}

// StartForSuite boots a System with no testing.TB attached, for suites that
// share one system across many test functions via TestMain. Callers must
// call the returned System's Close (or the returned cleanup func) when done.
func StartForSuite() (*System, error) {
	dir, err := os.MkdirTemp("", "notify-eval-*")
	if err != nil {
		return nil, err
	}
	sys, err := boot(dir)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	innerTeardown := sys.teardown
	sys.teardown = func() {
		innerTeardown()
		os.RemoveAll(dir)
	}
	return sys, nil
}

func boot(dir string) (*System, error) {
	store, err := rulestore.Open(filepath.Join(dir, "notify.db"))
	if err != nil {
		return nil, err
	}

	b := bus.NewMemory()

	ctx, cancel := context.WithCancel(context.Background())
	filterSvc := filter.New(b, store)
	filterDone := make(chan struct{})
	go func() {
		defer close(filterDone)
		filterSvc.Run(ctx)
	}()

	ingestorSrv := httptest.NewServer(ingestor.New(b))
	rulesSrv := httptest.NewServer(rules.New(b, store))
	deliverSrv := httptest.NewServer(deliver.New(b, fstest.MapFS{}))

	return &System{
		Bus:         b,
		Store:       store,
		IngestorURL: ingestorSrv.URL,
		RulesURL:    rulesSrv.URL,
		DeliverURL:  deliverSrv.URL,
		teardown: func() {
			cancel()
			ingestorSrv.Close()
			rulesSrv.Close()
			deliverSrv.Close()
			<-filterDone
			store.Close()
		},
	}, nil
}
