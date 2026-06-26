package evaluations

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"notify/pkg/contracts"
)

// apiKey is the MVP's one static credential (notify_mvp_plan.md section 5).
// It's part of the public auth contract, not an implementation detail, so
// the evaluations suite is entitled to know it.
const apiKey = "local-key"

func newUUID(t testing.TB) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("generating uuid: %v", err)
	}
	return id.String()
}

// ---- raw HTTP helpers ----

func authedRequest(t testing.TB, method, url string, body []byte) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

// httpPost posts a raw, unauthenticated-by-default body straight to a URL —
// used by the malformed-input contract test, which cares only about status
// codes.
func httpPost(t testing.TB, url, body string) *http.Response {
	t.Helper()
	return authedRequest(t, http.MethodPost, url, []byte(body))
}

// ---- ingestor (Capture BC) ----

// publishViaHTTP posts a notification to the ingestor and returns its id.
func publishViaHTTP(t testing.TB, n contracts.Notification) string {
	t.Helper()
	body, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	resp := authedRequest(t, http.MethodPost, sys.IngestorURL+"/notifications", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("publishViaHTTP: expected 202, got %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding ingestor response: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("publishViaHTTP: response contained no id")
	}
	return out.ID
}

// publishNotification is the property-test alias for publishViaHTTP.
func publishNotification(t testing.TB, n contracts.Notification) string {
	return publishViaHTTP(t, n)
}

// ---- rule-api (Matching BC) ----

type ruleWire struct {
	ID            string `json:"id,omitempty"`
	SourceApp     string `json:"source_app"`
	SourceAccount string `json:"source_account"`
	Title         string `json:"title"`
}

// setUserRule and setUserRules write directly into the shared rule store
// rather than going through the HTTP API. This is deliberate: it lets test
// setup express rules the HTTP API itself rejects as ambiguous (an
// all-fields-empty catch-all rule — see internal/rules/service.go), and
// keeps fixture setup fast. The HTTP API surface itself is exercised
// separately by createRuleViaHTTP/deleteRuleViaHTTP in contract_rules_test.go.
func setUserRule(t testing.TB, r contracts.Rule) {
	t.Helper()
	setUserRules(t, []contracts.Rule{r})
}

func setUserRules(t testing.TB, rules []contracts.Rule) {
	t.Helper()
	for _, r := range rules {
		if r.UserID == "" {
			r.UserID = "local"
		}
		if r.ID == "" {
			r.ID = newUUID(t)
		}
		if err := sys.Store.Create(r); err != nil {
			t.Fatalf("setUserRule: %v", err)
		}
	}
}

func clearAllRules(t testing.TB) {
	t.Helper()
	existing, err := sys.Store.List("local")
	if err != nil {
		t.Fatalf("clearAllRules: listing: %v", err)
	}
	for _, r := range existing {
		if _, err := sys.Store.Delete("local", r.ID); err != nil {
			t.Fatalf("clearAllRules: deleting %s: %v", r.ID, err)
		}
	}
}

func createRuleViaHTTP(t testing.TB, r contracts.Rule) string {
	t.Helper()
	body, err := json.Marshal(ruleWire{SourceApp: r.SourceApp, SourceAccount: r.SourceAccount, Title: r.Title})
	if err != nil {
		t.Fatalf("marshal rule: %v", err)
	}
	resp := authedRequest(t, http.MethodPost, sys.RulesURL+"/rules", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createRuleViaHTTP: expected 201, got %d", resp.StatusCode)
	}
	var out ruleWire
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding create-rule response: %v", err)
	}
	return out.ID
}

func deleteRuleViaHTTP(t testing.TB, id string) {
	t.Helper()
	resp := authedRequest(t, http.MethodDelete, sys.RulesURL+"/rules/"+id, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("deleteRuleViaHTTP: expected 204, got %d", resp.StatusCode)
	}
}

func listRulesViaHTTP(t testing.TB) []ruleWire {
	t.Helper()
	resp := authedRequest(t, http.MethodGet, sys.RulesURL+"/rules", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("listRulesViaHTTP: expected 200, got %d", resp.StatusCode)
	}
	var out []ruleWire
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding rules list: %v", err)
	}
	return out
}

func assertRuleExistsViaHTTP(t testing.TB, id string) {
	t.Helper()
	for _, r := range listRulesViaHTTP(t) {
		if r.ID == id {
			return
		}
	}
	t.Fatalf("rule %s not found via GET /rules", id)
}

func assertRuleAbsentViaHTTP(t testing.TB, id string) {
	t.Helper()
	for _, r := range listRulesViaHTTP(t) {
		if r.ID == id {
			t.Fatalf("rule %s still present via GET /rules", id)
		}
	}
}

func waitForRuleEvent(t testing.TB, ruleID string, kind contracts.RuleChangedKind, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if ruleEventsRecorder.has(ruleID, kind) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("rule-changed event %s for rule %s not seen within %v", kind, ruleID, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---- stream assertions (Matching BC routing outcomes) ----

func assertPresentInStream(t testing.TB, id, topic string, timeout time.Duration) {
	t.Helper()
	if _, ok := recorderFor(topic).waitFor(id, timeout); !ok {
		t.Fatalf("expected notification %s on %s within %v, not found", id, topic, timeout)
	}
}

func assertAbsentFromStream(t testing.TB, id, topic string, timeout time.Duration) {
	t.Helper()
	if _, ok := recorderFor(topic).waitFor(id, timeout); ok {
		t.Fatalf("expected notification %s to be absent from %s, but it appeared", id, topic)
	}
}

func waitForInStream(t testing.TB, id, topic string, timeout time.Duration) bool {
	t.Helper()
	_, ok := recorderFor(topic).waitFor(id, timeout)
	return ok
}

func waitForMessageOnTopic(t testing.TB, topic, id string, timeout time.Duration) contracts.Notification {
	t.Helper()
	n, ok := recorderFor(topic).waitFor(id, timeout)
	if !ok {
		t.Fatalf("message %s not found on %s within %v", id, topic, timeout)
	}
	return n
}

// routingOutcome reports which of the two routing streams a notification
// ended up on — used by determinism property tests that compare outcomes
// across two separately-published notifications.
func observeRouting(t testing.TB, id string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if matchedRecorder.has(id) {
			return "matched"
		}
		if discardedRecorder.has(id) {
			return "discarded"
		}
		if time.Now().After(deadline) {
			t.Fatalf("notification %s did not appear on either stream within %v", id, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitForRoutingOutcome polls both routing streams concurrently (rather than
// waiting out a full timeout on each in sequence) and reports as soon as
// either is observed — needed by mutual-exclusivity checks, where exactly
// one of the two return values will only ever resolve by timing out.
func waitForRoutingOutcome(t testing.TB, id string, timeout time.Duration) (matched, discarded bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		matched = matchedRecorder.has(id)
		discarded = discardedRecorder.has(id)
		if matched || discarded {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertPresentInAtLeastOneStream(t testing.TB, id string, topics []string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		for _, topic := range topics {
			if recorderFor(topic).has(id) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("notification %s absent from all of %v within %v", id, topics, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ---- delivery-service (Delivery BC) SSE client ----

type sseClient struct {
	events chan contracts.Notification

	resp      *http.Response
	done      chan struct{}
	closeOnce sync.Once
}

// Close disconnects the SSE client and waits for its reader goroutine to
// exit. Idempotent. Property tests that open many short-lived connections in
// a loop must call this per-iteration rather than relying on t.Cleanup, which
// only runs at the end of the whole test function — leaving every prior
// connection open (and still receiving every later iteration's
// notifications) for the test's entire duration.
func (c *sseClient) Close() {
	c.closeOnce.Do(func() {
		c.resp.Body.Close()
		<-c.done
	})
}

// subscribeSSE connects to the delivery-service's SSE endpoint. The
// subscription is established server-side before this call returns (see
// internal/deliver/service.go: subscribe-before-200-OK), so any notification
// published after subscribeSSE returns is guaranteed to be observed.
func subscribeSSE(t testing.TB) *sseClient {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, sys.DeliverURL+"/events?token="+apiKey, nil)
	if err != nil {
		t.Fatalf("building SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connecting to SSE endpoint: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("SSE endpoint returned %d", resp.StatusCode)
	}

	c := &sseClient{
		events: make(chan contracts.Notification, 64),
		resp:   resp,
		done:   make(chan struct{}),
	}
	go func() {
		defer close(c.done)
		defer close(c.events)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			var n contracts.Notification
			if err := json.Unmarshal([]byte(data), &n); err != nil {
				continue
			}
			// Never block on a stalled/abandoned consumer — a stuck send
			// here would prevent this goroutine from ever noticing Close().
			select {
			case c.events <- n:
			default:
			}
		}
	}()

	t.Cleanup(c.Close)

	return c
}

func waitForSSEEventWithID(t testing.TB, c *sseClient, id string, timeout time.Duration) contracts.Notification {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case n, ok := <-c.events:
			if !ok {
				t.Fatalf("SSE stream closed before event %s arrived", id)
			}
			if n.ID == id {
				return n
			}
		case <-timer.C:
			t.Fatalf("SSE event %s not received within %v", id, timeout)
		}
	}
}

// ---- INV-7: subset/superset overlap detection ----

// fieldCovers reports whether field value r "covers" s: every value satisfying
// s as a matching criterion also satisfies r.
//   - "*" or any glob in r covers s if path.Match(r, s) is true
//   - exact r covers only itself
func fieldCovers(r, s string) bool {
	if r == s {
		return true
	}
	if strings.Contains(r, "*") {
		ok, _ := path.Match(r, s)
		return ok
	}
	return false
}

// ruleCovers reports whether rule r covers rule s: every notification matched
// by s is also matched by r, field by field.
func ruleCovers(r, s ruleWire) bool {
	return fieldCovers(r.SourceApp, s.SourceApp) &&
		fieldCovers(r.SourceAccount, s.SourceAccount) &&
		fieldCovers(r.Title, s.Title)
}

// rulesOverlap reports whether r and s are in a subset-or-superset
// relationship — i.e. one covers the other.
func rulesOverlap(r, s ruleWire) bool {
	return ruleCovers(r, s) || ruleCovers(s, r)
}

func collectSSEEventsWithID(t testing.TB, c *sseClient, id string, window time.Duration) []contracts.Notification {
	t.Helper()
	var got []contracts.Notification
	timer := time.NewTimer(window)
	defer timer.Stop()
	for {
		select {
		case n, ok := <-c.events:
			if !ok {
				return got
			}
			if n.ID == id {
				got = append(got, n)
			}
		case <-timer.C:
			return got
		}
	}
}
