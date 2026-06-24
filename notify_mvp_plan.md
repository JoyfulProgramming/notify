# Notify MVP — Phoenix Architecture Implementation Plan

> Architecture 1 (Deletion-Safe Grain) · Go throughout · Google Cloud Pub/Sub as the event bus
> Bounded Contexts: **Capture** (everything from apps) · **Matching** (rule evaluation) · **Delivery** (what the user sees)
> Status: Draft — for review and iteration before any code is written

---

## 0. What This Document Is

This is a pre-code planning artifact. Its job is to answer three questions before a single line of Go is written:

1. What must this system **do** — in plain English, as verifiable invariants and behavioral examples?
2. What are the **stable boundaries** that survive all code regenerations?
3. What is the **smallest system** that proves those boundaries work end-to-end?

Do not treat this as a specification to be fully implemented before getting feedback. It is a starting point for iteration. Read it, mark what feels wrong or incomplete, and we revise it before writing any code.

---

## 1. What We're Building (The MVP)

A minimal but real pipeline that:

1. **Accepts a notification from an app** (simulated via HTTP POST — no Android required yet) — in the **Capture** bounded context
2. **Routes it through Google Cloud Pub/Sub**
3. **Evaluates it against user-defined rules** (filter-service and rule-api — in the **Matching** bounded context)
4. **Delivers matched notifications to the user** via Server-Sent Events (SSE) — in the **Delivery** bounded context
5. **Supports basic rule management** (create, list, delete rules via HTTP API)

End-to-end flow:

```
── CAPTURE BOUNDED CONTEXT ─────────────────────────────────────
  A Notification here is what an app generates.
  It makes no claim about relevance to the user.

  HTTP POST /notifications
        │
        ▼
  notification-ingestor (Go)
        │  publishes notification.v1 to Pub/Sub
        ▼
  Pub/Sub topic: notifications.captured

── MATCHING BOUNDED CONTEXT ─────────────────────────────────────
  The Matching context owns whether a Notification qualifies for
  delivery. It holds the Rule aggregate and the evaluation logic.

        │
        ▼
  filter-service (Go, stateless)
    reads rules from: rule-store (SQLite)
        ├──▶ Pub/Sub topic: notifications.matched
        └──▶ Pub/Sub topic: notifications.discarded  (did not qualify)

  rule-api (Go, HTTP)
    reads/writes: rule-store (SQLite)
    emits: rule-changed events to Pub/Sub topic: rules.changed

── DELIVERY BOUNDED CONTEXT ────────────────────────────────────
  A Notification here has cleared the Matching context.
  It makes an explicit claim: this is worth the user's attention.

        notifications.matched
                │
                ▼
          delivery-service (Go)
                │  pushes over SSE
                ▼
          Browser (minimal HTML page)
```

This is the smallest system that can prove all the core invariants. It is real software, not a prototype. The conserved layer is the same one that will survive into v2, v3, and beyond.

---

## 2. What We Are NOT Building Yet

The following are explicitly out of scope for the MVP. They are listed so the plan stays honest.

| Out of scope | Why |
|---|---|
| Android notification listener | Fast layer — add after the cloud pipeline is stable |
| Projects and schedules | Core domain complexity — adds significantly to the rule schema; add in v2 |
| Contacts, groups, specificity rules | Same — powerful domain logic, but layered on top of basic rules |
| FCM push (mobile delivery) | Add after SSE delivery is working |
| Authentication / multi-user | Start with a single hardcoded `user_id = "local"` |
| Focus mode / batching | Post-MVP feature |
| Tasks / Intend integration | Post-MVP feature |
| Notification importance / Eisenhower classification (Capture BC) | Add in v2 once projects exist |

None of these invalidate the architecture. The conserved layer is designed to accommodate them as additions, not changes.

---

## 3. Architecture

Every component in the MVP must pass the **deletion test**: can it be completely regenerated from its one-sentence spec plus the shared invariants? If the answer is no, either the spec is wrong or the component is too large.

The components are deliberately separated by the Pub/Sub boundary. A developer who has never seen the filter-service code can replace it entirely — as long as the replacement subscribes to `notifications.captured` and publishes to `notifications.matched` or `notifications.discarded` according to the same contracts.

### Bounded Contexts

```
CAPTURE CONTEXT ─────────────────────────────────────────────────────────
  Ubiquitous language: notification, source app, ingestion, deduplication
  A Notification is what an app generates — title, body, source app,
  device timestamp. It makes no claim about relevance to the user.

  Components:        notification-ingestor
  Published language: notification.v1 on notifications.captured topic

                    │  ← Capture→Matching boundary (notifications.captured)
                    ▼

MATCHING CONTEXT ────────────────────────────────────────────────────────
  Ubiquitous language: rule, match, specificity, evaluation
  The Matching context owns whether a Notification qualifies for delivery.
  It holds the Rule aggregate and the evaluation logic. It borrows the
  Notification type from the conserved layer — it does not own it.

  Components:        filter-service, rule-api
  Published language: notification.v1 on notifications.matched topic
                      RuleChangedEvent on rules.changed topic

                    │  ← Matching→Delivery boundary (notifications.matched)
                    ▼

DELIVERY CONTEXT ────────────────────────────────────────────────────────
  Ubiquitous language: notification, delivery, acknowledgement
  A Notification here has cleared the Matching context.
  It makes an explicit claim: this is worth the user's attention right now.

  Components:        delivery-service
  Published language: notification.v1 on notifications.matched topic
```

The same word — **Notification** — appears in all three contexts. The Pub/Sub topic is what makes the meaning unambiguous:

- `notifications.captured`: a Capture-context Notification — what an app generated
- `notifications.matched`: a Delivery-context Notification — cleared by the Matching context
- `notifications.discarded`: a Notification that the Matching context ruled out

**Why rule-api belongs in the Matching bounded context:**

The Rule is the core aggregate of the Matching context. The filter-service evaluates it; the rule-api manages its lifecycle. Both services speak the Matching context's ubiquitous language: rule, specificity, match. Placing rule-api in Delivery would split the Rule aggregate's ownership across contexts — filter-service would own evaluation but not lifecycle, with no clear home for the domain.

**Why the filter-service is not a pure context mapper:**

A pure context mapper has no domain model of its own — it only translates between two contexts. The filter-service does translate at the Capture boundary (reading Capture's Notification), but it also owns evaluation logic using Matching's Rule aggregate. That makes it a full member of the Matching bounded context, not a stateless translator. The adapter role at the Capture→Matching boundary is a responsibility it carries, not its identity.

### Pace Layers

```
SLOW LAYER (almost never changes — the conserved boundary)
  pkg/contracts/notification.go  — notification.v1 schema as Go struct + JSON tags
  pkg/contracts/rule.go          — Rule schema as Go struct
  pkg/contracts/events.go        — rule-changed event schema
  Pub/Sub topic names            — notifications.captured, notifications.matched,
                                   notifications.discarded, rules.changed

MID LAYER (changes monthly — the services)
  cmd/ingestor/     — HTTP → Pub/Sub                    [Capture BC]
  cmd/filter/       — Pub/Sub → rule evaluation → Pub/Sub [Matching BC]
  cmd/rules/        — Rule CRUD HTTP API                 [Matching BC]
  cmd/deliver/      — Pub/Sub → SSE                     [Delivery BC]

FAST LAYER (changes weekly or daily)
  web/              — minimal HTML/SSE page for receiving notifications
  cmd/deliver/ SSE handler — adapts as browser requirements change
```

---

## 4. Repository Structure

```
notify/
├── pkg/
│   └── contracts/
│       ├── notification.go    # notification.v1 — the slow layer
│       ├── rule.go            # Rule schema — the slow layer
│       └── events.go          # rule-changed event
│
├── cmd/
│   ├── ingestor/
│   │   └── main.go
│   ├── filter/
│   │   └── main.go
│   ├── rules/
│   │   └── main.go
│   └── deliver/
│       └── main.go
│
├── internal/
│   ├── pubsubclient/          # thin wrapper, not conserved layer
│   └── rulestore/             # SQLite adapter
│
├── web/
│   └── index.html             # minimal SSE display page
│
├── evaluations/               # durable evaluations — contract tests and property tests
│   ├── contract_ingestor_test.go
│   ├── contract_filter_test.go
│   ├── contract_rules_test.go
│   ├── contract_delivery_test.go
│   └── property_test.go
│
├── docker-compose.yml         # Pub/Sub emulator + SQLite volume
├── go.mod
└── go.sum
```

The `evaluations/` directory contains only durable evaluations — tests that speak exclusively in public contracts. These do not import any `cmd/` or `internal/` package. They are the evaluations that survive any code regeneration.

---

## 5. The Conserved Layer (pkg/contracts)

These structs are the slow layer. Changing them requires a new schema version. Do not change them without explicit decision-making.

### notification.v1

```go
// pkg/contracts/notification.go

// Notification is the published language crossing all three bounded context boundaries.
// In the Capture context (notifications.captured topic): what an app generated.
// In the Delivery context (notifications.matched topic): what the user will see.
// Schema version: v1. Do not modify fields without versioning.
type Notification struct {
    ID              string            `json:"id"`               // UUID v7, assigned at ingest, immutable, system-wide dedup key
    UserID          string            `json:"user_id"`          // set by ingestor from authenticated token; trusted by all downstream services
    SourceApp       string            `json:"source_app"`       // e.g. "com.whatsapp", "com.google.gmail"
    SourceAccount   string            `json:"source_account"`   // account within the app, e.g. "john@work.com"; empty = unspecified
    SourceID        string            `json:"source_id"`        // item identifier in the source system, e.g. Gmail message ID; empty = not available
    SentBy          string            `json:"sent_by"`          // raw identifier of who sent it: phone, email, username, bot ID; empty = unspecified
    SentIn          string            `json:"sent_in"`          // group/channel/thread context, e.g. WhatsApp group ID; empty = direct/none
    Title           string            `json:"title"`            // may be empty
    Body            string            `json:"body"`             // may be empty
    DeviceID        string            `json:"device_id"`        // UUID identifying the source device
    DeviceTimestamp time.Time         `json:"device_timestamp"` // when the notification appeared on device
    ReceivedAt      time.Time         `json:"received_at"`      // when this system first ingested it
    Metadata        map[string]string `json:"metadata"`         // arbitrary key-value; no service may fail on unknown keys
}
```

**Plain-language contract:**
- `ID` is assigned once at ingestion and never changes. It is the deduplication key for the entire system.
- `UserID` is set by the ingestor from the authenticated caller's token. Downstream services (filter, delivery) trust it without re-authenticating. No service may accept a Notification with an absent or empty `user_id`.
- `SourceAccount` is the account identifier within the source app (e.g. `"john@work.com"` for work Gmail, `"john@personal.com"` for personal Gmail). Empty means the app has only one account or the account is unknown. Together with `SourceApp` it uniquely identifies the origin.
- `SourceID` is the identifier for this specific item in the source system — a Gmail message ID, a GitHub notification ID, a Jira issue key. Empty means the source app does not expose a stable identifier. It is an opaque string; its format is implicit in `SourceApp`. Logic for constructing deep links or actions from it belongs in the fast layer (UI/delivery adapters), not here.
- `SentBy` is the raw identifier of who sent the notification — a phone number, email address, username, or system identifier. Its format is implicit in `SourceApp`. Empty means unknown or a system notification with no individual sender.
- `SentIn` is the raw identifier of the group, channel, or thread context the notification came from. Empty means a direct message or no group context. Together with `SentBy` this enables the full specificity rule set (sender+group, sender-only, group-only) in v2.
- `DeviceTimestamp` reflects what the device (or HTTP client) saw. `ReceivedAt` is when the ingestor set it. Both are always present.
- No service may return an error or panic on encountering an unknown `metadata` key.
- Adding new optional fields is backward-compatible. Removing or renaming fields requires `notification.v2`.

### Rule schema

```go
// pkg/contracts/rule.go

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
//   ""                 empty   — ignore this field; matches any value
//   "com.google.gmail" exact   — field must equal this value exactly
//   "com.google.*"     pattern — field must match this glob pattern (* = any chars)
//
// Examples:
//   SourceApp: ""              matches any app
//   SourceApp: "com.whatsapp"  matches WhatsApp only
//   SourceApp: "com.google.*"  matches any Google app
//   SourceAccount: ""          matches any account
//   SourceAccount: "*@work.com" matches any work email account
//   Title: ""                  matches any title
//   Title: "*invoice*"         matches any title containing "invoice"
//   Title: "Invoice ready"     matches this exact title only
//
// No Priority field. When multiple rules match, the most specific rule wins.
// Specificity: exact > pattern > empty, and more fields populated > fewer. See INV-6.
// Note: SentBy and SentIn matching (contact+group specificity rules) are v2.
// The fields exist on Notification now so the schema does not need versioning when rules are added.
```

### rule-changed event

```go
// pkg/contracts/events.go

type RuleChangedKind string

const (
    RuleCreated RuleChangedKind = "CREATED"
    RuleUpdated RuleChangedKind = "UPDATED"
    RuleDeleted RuleChangedKind = "DELETED"
)

// RuleChangedEvent is published to rules.changed topic on any rule mutation.
type RuleChangedEvent struct {
    EventID   string          `json:"event_id"`   // UUID
    Kind      RuleChangedKind `json:"kind"`
    Rule      Rule            `json:"rule"`
    ChangedAt time.Time       `json:"changed_at"`
}
```

### Pub/Sub topic names (the boundary)

```
notifications.captured    — Capture→Matching boundary: notifications as apps generated them (published by ingestor)
notifications.matched     — Matching→Delivery boundary: notifications that cleared the rules (published by filter-service)
notifications.discarded   — Matching context: notifications that did not qualify (published by filter-service)
rules.changed             — Matching context: RuleChangedEvent stream (published by rule-api)
notifications.captured-deadletter — operational backstop: messages that exceeded max delivery
                                     attempts on the notifications.captured subscription (filter-service
                                     could not reach the rule store). Not a routing outcome — see open
                                     question #4. Distinct from notifications.discarded, which means a
                                     rule decision was made and no rule matched.
```

These topic names are the conserved boundary. `notifications.captured` marks the Capture→Matching boundary; `notifications.matched` marks the Matching→Delivery boundary. Only the filter-service may publish to `notifications.matched` or `notifications.discarded`. Any service that publishes to or subscribes from these topics is implicitly depending on this contract.

### Authentication Architecture (Conserved Decision)

Authentication is decided here — in the conserved layer — because the pattern affects `user_id` on every Notification and the delivery-service fan-out model. Getting these wrong later requires a schema migration.

**The pattern: authenticate once at the boundary, propagate identity on the message.**

```
HTTP POST /notifications
  Authorization: Bearer <token>        ← auth happens here, once
        │
        ▼
  notification-ingestor
  validates token → resolves user_id
  stamps user_id on every Notification
        │
        ▼
  notifications.captured  { "id": "...", "user_id": "abc123", ... }
        │                               ↑ identity travels with the message
        ▼
  filter-service: loads rules WHERE user_id = "abc123"   (already works)
        │
        ▼
  notifications.matched  { "id": "...", "user_id": "abc123", ... }
        │
        ▼
  delivery-service: only delivers to SSE connection authenticated as "abc123"
```

The ingestor is the only service that performs authentication. Downstream services filter by `user_id` — they do not re-authenticate.

**Delivery-service fan-out model (locked in now):**

When multiple users are connected, the delivery-service must not fan out all `notifications.matched` messages to all clients. The correct approach is **Pub/Sub message filtering on the `user_id` attribute** — Google Cloud Pub/Sub supports per-subscription filter expressions natively. Each user's SSE session uses a subscription filtered to `attributes.user_id = "<id>"`. This scales without per-user topics.

```
notifications.matched topic
  subscription: deliver-user-abc123  filter: attributes.user_id="abc123"
  subscription: deliver-user-def456  filter: attributes.user_id="def456"
```

The ingestor must therefore set `user_id` as both a JSON field on the message body AND as a Pub/Sub message attribute when publishing. This is a publishing contract, not just a schema contract.

**The SSE endpoint also authenticates:**

`GET /events` (the SSE upgrade) must carry a token. The delivery-service validates it at connection time, resolves `user_id`, and creates or attaches to the appropriate filtered subscription. The token only needs to be validated once per connection, not per message.

**MVP implementation (trivially simple, architecturally complete):**

```
API key in Authorization header
  → static map: {"local-key": "local"}
  → user_id = "local"
```

The infrastructure is real (middleware validates the header, `user_id` is stamped, subscriptions are filtered) but the implementation is a one-liner. Swapping in JWT validation changes only the token-validation function — nothing else in the architecture moves.

---

## 6. System Invariants

These must hold across **any** implementation, in any language, across any regeneration cycle. They are the constitution of the system. Every contract test and property test maps to one of these.

```
INV-1: A notification that enters the Capture context is never silently lost.
       It is either present in notifications.matched (Delivery context),
       present in notifications.discarded, or causes an explicit error.
       It is never simply absent.

INV-2: User filtering rules are the single source of truth for what crosses
       into the Delivery context. No implementation may place a notification
       on notifications.matched unless at least one rule matches it.

INV-3: A notification in the Delivery context is surfaced at most once per SSE
       client per session. Duplicates are suppressed by id.

INV-4: The filter-service (Matching BC) is deterministic: given the same
       Capture-context notification and the same set of active rules, it always
       produces the same routing decision.

INV-5: A rule change takes effect for all future notifications immediately.
       It does not retroactively promote already-discarded notifications into
       the Delivery context.

INV-6: In v1, all rules deliver. If any rule matches a notification, it is
       delivered. Multiple matching rules cannot conflict — they all say the
       same thing. Specificity resolution is therefore not needed in v1.

       Specificity rules are documented here for v2, when DISCARD (exception)
       rules are introduced and conflicts become possible:

       1. More fields populated = more specific:
            SourceApp + SourceAccount + Title  (most specific)
            SourceApp + SourceAccount
            SourceApp + Title
            SourceApp only
            All fields empty                   (least specific, matches everything)

       2. Within the same field, exact > pattern > empty:
            "com.google.gmail"   exact
            "com.google.*"       pattern
            ""                   empty

       These two dimensions combine. No user-assigned priority number ever
       exists — specificity is always derived from the rule's shape alone.
```

---

## 7. notification-ingestor (Capture BC)

**Spec:** Accepts a JSON notification body via `POST /notifications`, authenticates the caller and stamps `user_id`, assigns an `id` (UUID v7) and `received_at` timestamp if absent, validates the message against `notification.v1`, and publishes it to the `notifications.captured` Pub/Sub topic with `user_id` as a message attribute.

**Invariants:** INV-1

### Behavioral Examples

```
BEHAVIOR: Basic ingestion publishes to notifications.captured
  Given the ingestor is running
  When I POST {"source_app": "com.whatsapp", "title": "Alice: hey", "body": "Are you free?"} to /notifications
  Then the response status is 202
  And the response body contains a "id"
  And a message appears on the notifications.captured Pub/Sub topic within 2 seconds
  And that message contains the same id from the response

BEHAVIOR: Ingestor rejects malformed input
  Given the ingestor is running
  When I POST an empty body to /notifications
  Then the response status is 400

BEHAVIOR: Ingestor assigns received_at
  Given the ingestor is running
  When I POST a notification without a received_at field
  Then the published message's received_at is within 1 second of the current time

BEHAVIOR: Ingestor deduplicates by id
  Given a notification with id "abc-123" has already been published to the Capture context
  When I POST the same id again
  Then only one message appears on notifications.captured
```

### Contract Tests

```go
// evaluations/contract_ingestor_test.go

// TestContract_IngestorAssignsReceivedAt maps to the schema contract.
func TestContract_IngestorAssignsReceivedAt(t *testing.T) {
    before := time.Now()
    id := publishViaHTTP(t, Notification{SourceApp: "com.whatsapp", Title: "Test"})
    msg := waitForMessageOnTopic(t, "notifications.captured", id, 3*time.Second)
    after := time.Now()

    if msg.ReceivedAt.Before(before) || msg.ReceivedAt.After(after) {
        t.Fatalf("received_at %v is outside the expected window [%v, %v]", msg.ReceivedAt, before, after)
    }
}

// TestContract_IngestorRejectsMalformed
func TestContract_IngestorRejectsMalformed(t *testing.T) {
    resp := httpPost(t, "/notifications", `{}`)
    if resp.StatusCode != 400 {
        t.Fatalf("expected 400 for empty body, got %d", resp.StatusCode)
    }
}
```

### Provenance

```
Why it exists:
  The entry point for the Capture bounded context and the single auth boundary.
  Its job: authenticate the caller, stamp user_id, assign identity (id, received_at),
  validate against notification.v1, and publish to the Capture-context topic
  (notifications.captured) with user_id set as both a message body field and a Pub/Sub
  message attribute.
  Separating this from the filter-service means the filter-service can be
  regenerated freely without any risk of losing or mangling incoming notifications.

Rejected alternatives:
  Filter directly on write (ingestor = filter): Couples validation to business logic.
  Regenerating the filter would also touch the ingest path, increasing blast radius.

  Let clients publish directly to Pub/Sub: Removes the validation boundary.
  Any client with a Pub/Sub key could publish malformed or malicious messages.

Active assumptions:
  A single ingestor instance is sufficient for MVP; horizontal scaling comes later.
  HTTP is the ingestion protocol for MVP (not gRPC, not Android Intents).
  id uniqueness is enforced by deduplication, not by a uniqueness constraint.

What would make this wrong:
  If the Android listener must publish directly to Pub/Sub (bypassing the ingestor),
  the validation contract needs to live somewhere else — perhaps in a Pub/Sub schema
  registry rather than in application code.
```

---

## 8. filter-service (Matching BC)

**Spec:** Subscribes to `notifications.captured`, loads the owner's rules from the rule store, and publishes the notification to `notifications.matched` if any rule matches, or `notifications.discarded` if none match.

**Invariants:** INV-1, INV-2, INV-4, INV-5, INV-6

### Behavioral Examples

```
BEHAVIOR: Notification matching a rule is delivered
  Given the user has a rule: {source_app: "com.whatsapp"}
  When a notification with source_app "com.whatsapp" arrives on notifications.captured
  Then within 3 seconds the notification appears on notifications.matched
  And it does not appear on notifications.discarded

BEHAVIOR: Notification with no matching rule is discarded
  Given the user has no rules for source_app "com.instagram"
  When a notification with source_app "com.instagram" arrives on notifications.captured
  Then the notification appears on notifications.discarded
  And it does not appear on notifications.matched

BEHAVIOR: Catch-all rule (all fields empty) matches any notification
  Given the user has a rule: {source_app: ""}
  When a notification with source_app "com.example.anything" arrives on notifications.captured
  Then the notification appears on notifications.matched

BEHAVIOR: Pattern rule matches on glob
  Given the user has a rule: {source_app: "com.google.*"}
  When a notification with source_app "com.google.gmail" arrives
  Then it appears on notifications.matched
  When a notification with source_app "com.whatsapp" arrives
  Then that notification appears on notifications.discarded

BEHAVIOR: Title pattern narrows matching
  Given the user has a rule: {source_app: "com.google.gmail", title: "*invoice*"}
  When a notification {source_app: "com.google.gmail", title: "Your invoice is ready"} arrives
  Then it appears on notifications.matched
  When a notification {source_app: "com.google.gmail", title: "Newsletter: weekly digest"} arrives
  Then that notification appears on notifications.discarded

BEHAVIOR: Rule change takes immediate effect
  Given no rules exist for source_app "com.slack"
  And a notification {source_app: "com.slack"} is sent and appears on notifications.discarded
  When I add a rule: {source_app: "com.slack"}
  And I send another notification {source_app: "com.slack"}
  Then the second notification appears on notifications.matched
  And the first notification remains on notifications.discarded (no retroactive change)
```

### Contract Tests

```go
// evaluations/contract_filter_test.go

// TestContract_MatchingRuleLeadsToDelivery maps to INV-2.
func TestContract_MatchingRuleLeadsToDelivery(t *testing.T) {
    setUserRule(t, Rule{SourceApp: "com.whatsapp"})
    id := publishViaHTTP(t, Notification{SourceApp: "com.whatsapp", Title: "Alice: hey"})
    assertPresentInStream(t, id, "notifications.matched", 5*time.Second)
}

// TestContract_NoMatchingRuleRoutesToDiscarded maps to INV-1, INV-2.
func TestContract_NoMatchingRuleRoutesToDiscarded(t *testing.T) {
    clearAllRules(t)
    id := publishViaHTTP(t, Notification{SourceApp: "com.twitter", Title: "Someone liked your tweet"})
    assertPresentInStream(t, id, "notifications.discarded", 5*time.Second)
    assertAbsentFromStream(t, id, "notifications.matched", 1*time.Second)
}

// TestContract_NoRuleDefaultsToDiscarded maps to INV-2 (rules are the source of truth).
func TestContract_NoRuleDefaultsToDiscarded(t *testing.T) {
    clearAllRules(t)
    id := publishViaHTTP(t, Notification{SourceApp: "com.example.unknown"})
    assertPresentInStream(t, id, "notifications.discarded", 5*time.Second)
}

// TestContract_RuleChangeAppliesToFutureOnly maps to INV-5.
func TestContract_RuleChangeAppliesToFutureOnly(t *testing.T) {
    clearAllRules(t)
    id1 := publishViaHTTP(t, Notification{SourceApp: "com.slack", Title: "Message from Dave"})
    assertPresentInStream(t, id1, "notifications.discarded", 5*time.Second)

    setUserRule(t, Rule{SourceApp: "com.slack"})

    id2 := publishViaHTTP(t, Notification{SourceApp: "com.slack", Title: "Another message"})
    assertPresentInStream(t, id2, "notifications.matched", 5*time.Second)

    // First notification remains discarded — no retroactive change
    assertAbsentFromStream(t, id1, "notifications.matched", 1*time.Second)
}
```

### Property Tests

```go
// evaluations/property_test.go

// PROPERTY: Filter service is deterministic.
// Maps to: INV-4
func TestProperty_FilterIsDeterministic(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        rules := generateArbitraryRuleSet(t)
        setUserRules(t, rules)

        id1 := publishNotification(t, n)
        outcome1 := observeRouting(t, id1, 5*time.Second)

        // Give the notification a new ID but identical content and same rules
        n2 := n
        n2.ID = newUUID()
        id2 := publishNotification(t, n2)
        outcome2 := observeRouting(t, id2, 5*time.Second)

        if outcome1 != outcome2 {
            t.Fatalf("same notification routed differently: first=%s second=%s", outcome1, outcome2)
        }
    })
}

// PROPERTY: Any matching rule delivers the notification.
// Maps to: INV-2, INV-6
func TestProperty_AnyMatchDelivers(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        rule := generateMatchingRule(t, n)
        setUserRule(t, rule)

        id := publishNotification(t, n)
        assertPresentInStream(t, id, "notifications.matched", 5*time.Second)
    })
}

// PROPERTY: Every notification is routed to exactly one of the two streams.
// Maps to: INV-1, INV-2
func TestProperty_MutuallyExclusiveRouting(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        setUserRules(t, generateArbitraryRuleSet(t))

        id := publishNotification(t, n)

        inMatched   := waitForInStream(t, id, "notifications.matched", 3*time.Second)
        inDiscarded := waitForInStream(t, id, "notifications.discarded", 3*time.Second)

        if inMatched && inDiscarded {
            t.Fatal("notification appeared in both streams — routing must be mutually exclusive")
        }
        if !inMatched && !inDiscarded {
            t.Fatal("notification appeared in neither stream — violates INV-1")
        }
    })
}
```

### Provenance

```
Why it exists:
  The core evaluation service of the Matching bounded context, and the
  boundary adapter at the Capture→Matching crossing.
  Its entire job: does this Notification from Capture become a Notification
  in Delivery? That question is answered by the user's rules.
  Filtering must happen in the cloud so that rule changes apply to all devices
  simultaneously and the audit log is centralised. If filtering were on-device,
  a rule change on the web UI would not affect Android until the next sync.

Rejected alternatives:
  On-device filtering only: Rules would diverge between devices.
  Filter inside the ingestor: Couples filtering to ingestion; prevents replaying
    historical events against new rules (a capability we want in v2).
  Filter inside the delivery service: Delivery should be dumb; routing logic
    and delivery logic have different failure modes and change at different speeds.

Active assumptions:
  Users have at most ~50 active rules in MVP; evaluating all and picking the
    most specific is fast enough at this scale.
  Specificity is calculated from rule shape alone — no user-assigned priority.
  Rule evaluation is stateless: no rule references another rule's output.
  Pub/Sub delivers each message at least once; deduplication is handled by
    id, not by the filter service itself.
  The rule store (SQLite) is local to the filter service in MVP; in production
    this would be a shared Postgres database or a rules cache.

What would make this wrong:
  If users need rule changes applied retroactively to historical events,
  the filter service needs a replay capability (subscribing to a time-windowed
  notifications.captured and re-routing through current rules).
  If rule count grows to thousands, evaluating all rules per notification is
    too slow and an indexed approach is needed.
```

---

## 9. rule-api (Matching BC)

**Spec:** Provides `POST /rules`, `GET /rules`, `DELETE /rules/{id}` HTTP endpoints for managing a user's filter rules in SQLite, and publishes a `RuleChangedEvent` to `rules.changed` on every successful mutation.

**Invariants:** INV-5

### Behavioral Examples

```
BEHAVIOR: Create a rule persists it and emits an event
  Given the rule API is running
  When I POST {source_app: "com.whatsapp"} to /rules
  Then the response status is 201
  And the response body contains a "id"
  And a GET /rules returns a list containing that rule
  And a RuleChangedEvent (kind: CREATED) appears on the rules.changed topic

BEHAVIOR: Delete a rule removes it and emits an event
  Given a rule with id "xyz" exists
  When I DELETE /rules/xyz
  Then GET /rules no longer returns rule "xyz"
  And a RuleChangedEvent (kind: DELETED) appears on the rules.changed topic

BEHAVIOR: Create rule with missing required field is rejected
  When I POST {} to /rules (empty body)
  Then the response status is 400
```

### Contract Tests

```go
// evaluations/contract_rules_test.go

// TestContract_CreateRuleEmitsEvent maps to INV-5.
func TestContract_CreateRuleEmitsEvent(t *testing.T) {
    id := createRuleViaHTTP(t, Rule{SourceApp: "com.whatsapp"})
    assertRuleExistsViaHTTP(t, id)
    waitForRuleEvent(t, id, RuleCreated, 3*time.Second)
}

// TestContract_DeleteRuleEmitsEvent maps to INV-5.
func TestContract_DeleteRuleEmitsEvent(t *testing.T) {
    id := createRuleViaHTTP(t, Rule{SourceApp: "com.whatsapp"})
    deleteRuleViaHTTP(t, id)
    assertRuleAbsentViaHTTP(t, id)
    waitForRuleEvent(t, id, RuleDeleted, 3*time.Second)
}
```

### Provenance

```
Why it exists:
  Rules are the core aggregate of the Matching context — they define what qualifies for delivery.
  Rules must be manageable without restarting any services. The rule API is the
  single point of mutation for rule state, which means audit events (rule-changed)
  are always emitted and the filter service can stay stateless (pull from DB on
  each evaluation, or refresh a cache on rule-changed events).

Rejected alternatives:
  Manage rules via config file: Cannot change rules at runtime without redeployment.
  Let the filter service own rule mutation: Couples two responsibilities that change
    at different times (CRUD logic vs. evaluation logic).

Active assumptions:
  user_id = "local" for MVP — no authentication, no multi-tenancy.
  SQLite is sufficient for MVP; Postgres swap is a mid-layer change, not a conserved
    layer change, so it does not affect contracts or evaluations.

What would make this wrong:
  If multiple services need to write rules concurrently, SQLite's single-writer
  model is a bottleneck. Postgres + advisory locks or an event-sourced rule store
  would be needed.
```

---

## 10. delivery-service (Delivery BC)

**Spec:** Subscribes to `notifications.matched`, deduplicates by `id` per connected client, and streams matching notifications to all active SSE connections as JSON-encoded events.

**Invariants:** INV-3

### Behavioral Examples

```
BEHAVIOR: Notification on filtered stream reaches SSE client
  Given a browser is connected to the SSE endpoint /events
  When a notification appears on notifications.matched
  Then within 3 seconds the browser's SSE connection receives that notification as a JSON event
  And the event contains the id, source_app, title, and body

BEHAVIOR: Notification is not delivered twice to the same client in a session
  Given a browser is connected to /events
  When the same id appears on notifications.matched twice
  Then the browser receives that notification exactly once

BEHAVIOR: Multiple connected clients each receive the notification
  Given two browsers are connected to /events
  When a notification appears on notifications.matched
  Then both browsers receive the notification within 3 seconds
```

### Contract Tests

```go
// evaluations/contract_delivery_test.go

// TestContract_FilteredNotificationReachesSSEClient maps to the end-to-end pipeline.
func TestContract_FilteredNotificationReachesSSEClient(t *testing.T) {
    setUserRule(t, Rule{SourceApp: "com.whatsapp"})
    events := subscribeSSE(t)

    id := publishViaHTTP(t, Notification{SourceApp: "com.whatsapp", Title: "Hello"})
    event := waitForSSEEventWithID(t, events, id, 5*time.Second)

    if event.ID != id {
        t.Fatalf("expected id %s, got %s", id, event.ID)
    }
}
```

### Property Tests

```go
// evaluations/property_test.go

// PROPERTY: Deduplication by id.
// Maps to: INV-3
func TestProperty_DeduplicationByID(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryDeliverableNotification(t)
        clientEvents := subscribeSSE(t)

        count := rapid.IntRange(2, 5).Draw(t, "count")
        for i := 0; i < count; i++ {
            publishNotification(t, n) // same id each time
        }

        seen := collectSSEEventsWithID(t, clientEvents, n.ID, 5*time.Second)
        if len(seen) != 1 {
            t.Fatalf("expected exactly 1 delivery, got %d", len(seen))
        }
    })
}
```

### Provenance

```
Why it exists:
  The outward face of the Delivery context. It takes notifications that have
  cleared the Matching context and surfaces them to the user.
  Delivery is a fast layer that adapts to the delivery channel (SSE, FCM, WebSocket).
  Separating delivery from filtering means the filter service can be regenerated
  without touching any client-facing code, and new delivery channels (FCM for
  Android) can be added as additions, not changes.

Rejected alternatives:
  Filter service publishes directly to SSE: Couples a stateless cloud process to
  a stateful client connection. SSE connections require long-lived HTTP connections;
  mixing this into a PubSub subscriber changes the operational model significantly.
  Deliver from the ingestor: The ingestor should not know about connected clients.

Active assumptions:
  SSE is the only delivery channel in MVP.
  Each SSE connection authenticates at upgrade time; user_id is resolved once per connection.
  Each connected user gets a Pub/Sub subscription filtered to their user_id attribute.
  Deduplication (id) is tracked in memory per SSE connection.
  If the SSE connection drops, the client reconnects and may miss notifications
  that arrived while disconnected. Persistence / replay is a v2 feature.

What would make this wrong:
  If clients require guaranteed delivery (no missed notifications across reconnects),
  a durable per-client queue (Redis, Pub/Sub subscription per client) is needed.
```

---

## 11. System-Wide Property Tests

These properties span multiple components and cannot be attributed to any single service.

```go
// evaluations/property_test.go

// PROPERTY: No notification matching a rule is ever silently absent.
// Maps to: INV-1, INV-2
//
// For any arbitrarily generated notification, if at least one rule matches,
// the notification must appear in notifications.matched.
// It must never be absent from both notifications.matched AND notifications.discarded.
func TestProperty_NoSilentDiscard(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        rule := generateMatchingDeliverRule(t, n)
        setUserRule(t, rule)

        id := publishNotification(t, n)

        assertPresentInAtLeastOneStream(t, id,
            []string{"notifications.matched", "notifications.discarded"},
            10*time.Second,
        )
    })
}
```

---

## 12. Implementation Order

Follow the Phoenix sequence: invariants and contracts first, code last.

### Pre-code (days 1–2)

- [ ] Review and agree on this plan document. Revise until it feels right.
- [ ] Set up the development environment (see section 13).
- [ ] Commit `pkg/contracts/` — the slow layer — before anything else.
- [ ] Write the evaluation test files (`evaluations/`) as failing tests. These are the acceptance criteria. Do not write services until these exist.

### Week 1 — Core pipeline

- [ ] **filter-service**: Pure function `(Notification, []Rule) → matched | no match`. Easiest to specify completely. Pass the contract tests.
- [ ] **notification-ingestor**: HTTP handler + Pub/Sub publisher. Wire it to the local Pub/Sub emulator. Confirm the filter-service receives notifications from it.
- [ ] **rule-api**: SQLite-backed CRUD + rule-changed events. Confirm the filter-service reflects rule changes without restart.

### Week 2 — Delivery and end-to-end

- [ ] **delivery-service**: Pub/Sub subscriber + SSE handler. Get end-to-end delivery working to a minimal HTML page.
- [ ] End-to-end test: POST /notifications → browser receives SSE event.
- [ ] Run all contract tests green.
- [ ] Run property tests green.

### Week 3 — Operational readiness

- [ ] Live evaluation: delivery latency from `device_timestamp` to SSE receipt, logged.
- [ ] Dead-letter logging: anything on `notifications.discarded` is logged with reason.
- [ ] Duplicate delivery tracking: log any SSE event delivered more than once per client.
- [ ] `docker-compose up` starts the full system in one command.

### The n=1 test at day 14

Could a new Go developer, given only the specs, invariants, and evaluations (not the code), regenerate the filter-service from scratch, run it, and pass all contract and property tests? If yes: the MVP is complete. If no: improve the specs, not the code.

---

## 13. Development Environment

### Prerequisites

- Go 1.22+
- Docker (for Pub/Sub emulator)
- `gcloud` CLI or just the emulator Docker image

### Local Pub/Sub emulator

```yaml
# docker-compose.yml
services:
  pubsub-emulator:
    image: gcr.io/google.com/cloudsdktool/google-cloud-cli:emulators
    command: gcloud beta emulators pubsub start --host-port=0.0.0.0:8085
    ports:
      - "8085:8085"
    environment:
      - PUBSUB_PROJECT_ID=notify-local
```

Set in all services:

```
PUBSUB_EMULATOR_HOST=localhost:8085
PUBSUB_PROJECT_ID=notify-local
```

The Go Pub/Sub client automatically uses the emulator when `PUBSUB_EMULATOR_HOST` is set.

### Running the full stack locally

```
docker-compose up -d          # start Pub/Sub emulator
go run ./cmd/rules/           # starts rule-api on :8081
go run ./cmd/ingestor/        # starts ingestor on :8080
go run ./cmd/filter/          # starts filter subscriber
go run ./cmd/deliver/         # starts delivery-service with SSE on :8082
open web/index.html           # or serve it on :8083
```

### Running evaluations

```
go test ./evaluations/... -v -timeout 60s
```

Evaluations require all services running (they are integration tests against the real pipeline). Set `NOTIFY_TEST_INGESTOR=http://localhost:8080` etc. in your shell.

---

## 14. Live Evaluations (Invariant Monitoring)

These run in the background during development and will become production monitors.

| Metric | What it measures | Alert threshold |
|---|---|---|
| Delivery latency | `device_timestamp` → SSE receipt (p99) | > 5 seconds |
| Dead-letter rate | % of notifications on `notifications.discarded` vs. total | > 80% (indicates rules are too restrictive or misconfigured) |
| Duplicate SSE delivery | Any id delivered > 1× to a client | Any occurrence |
| Rule API error rate | 5xx responses on /rules | > 1% |
| Pub/Sub consumer lag | Age of oldest unprocessed message on notifications.captured | > 10 seconds |

---

## 15. Definition of Done (MVP)

The MVP is done when all of the following are true:

- [ ] All 6 system invariants (section 6) are verified by at least one contract test or property test.
- [ ] All behavioral examples (sections 7–10) pass as automated tests.
- [ ] The n=1 test passes: a new Go developer can regenerate the filter-service from its spec and evaluations without reading the existing code.
- [ ] `docker-compose up` + `go run ./cmd/...` (five commands) starts the full system.
- [ ] Posting a notification via `curl -X POST localhost:8080/notifications` results in an SSE event appearing in the browser within 5 seconds.
- [ ] A rule change via the API takes effect for the next signal posted, without restarting any service.
- [ ] The deletion test passes for each component: its one-sentence spec is sufficient to regenerate it.

---

## 16. Conserved-Layer Decisions (Resolved)

These were open questions affecting the conserved layer. All are now decided; revisit only via explicit conversation, not silently in code.

1. **Topic naming convention**: Flat kebab-case (`notification-raw`), dot-namespaced hierarchy (`notify.notification.raw`), or DDD-style past-tense events? Whatever we choose becomes a conserved boundary. Decision: DDD-style past-tense events — `notifications.captured`, `notifications.matched`, `notifications.discarded`, `rules.changed`. Chosen because it encodes the bounded-context language (capture/match/discard/rule-changed) directly in the topic name, and because renaming `filtered` → `matched` removes a real ambiguity in the original draft — "filtered" could be misread as "filtered out," which is the opposite of what the topic carries.

2. **Rule store startup**: Does the filter-service query SQLite on every message, or does it maintain an in-memory cache refreshed on `rules.changed`? Decision: query SQLite on every message. Simpler for MVP — no cache invalidation or staleness window to reason about, and SQLite reads are fast at MVP scale (~50 rules/user). A cache can be introduced later as a mid-layer change if latency demands it; it does not affect the conserved layer.

3. **Notification id assignment**: Does the HTTP client supply the id, or does the ingestor always generate it? The schema says "assigned at ingest" but should we allow the client to supply one (for idempotent retries)? Decision: allow client-supplied IDs; ingestor generates one if absent.

4. **Error handling on filter failure**: If the filter-service cannot reach SQLite, does it nack the Pub/Sub message (causing retry) or ack it and route to dead-letter? Decision: nack and let Pub/Sub redeliver. This preserves INV-1 and INV-2 — a notification must never land on `notifications.discarded` because of an infrastructure failure; `discarded` means a rule decision was made and no rule matched, not "we couldn't tell." Pub/Sub's subscription-level dead-letter policy (max delivery attempts) is the backstop if SQLite stays down — those messages go to a true dead-letter topic for operator attention, not `notifications.discarded`.

5. **SSE reconnection**: When an SSE client reconnects, does it replay recent notifications or start fresh? For MVP: start fresh. Replay is v2.

6. **Local SQLite file location**: Shared between rule-api and filter-service, or does each have its own copy synced via rules.changed? Decision: shared file at a known path in dev. In production this becomes Postgres.

---

## Appendix A — Domain Concepts and Their MVP Mapping

For reference, here is how the full Notify domain (from goals.md) maps to the MVP scope.

| Domain concept | In MVP? | Notes |
|---|---|---|
| Notification (Capture BC) | Yes — `Notification` struct in pkg/contracts on `notifications.captured` | What apps generate. Simplified: no sender, project, or priority attributes yet |
| Notification (Delivery BC) | Yes — `Notification` struct in pkg/contracts on `notifications.matched` | What the user sees. Same struct, different bounded context |
| Rule | Yes — source_app, source_account, title (exact, pattern, or empty). A rule means "surface this." | No specificity resolution, no DISCARD exception rules, no contact+group rules yet |
| Project | No | Post-MVP; adds schedule-based delivery |
| Schedule | No | Post-MVP; affects when notifications.matched events are actually pushed to SSE |
| Contact / Sender | No | Post-MVP; part of specificity rule system |
| Group | No | Post-MVP |
| Priority / Eisenhower matrix | No | Post-MVP; starts as rule attribute on Signal |
| Task / Intend integration | No | Post-MVP |
| Focus mode / batching | No | Post-MVP |
| Android listener | No | Fast layer; add after cloud pipeline is stable |
| Multi-user / auth | No | user_id = "local" throughout |

The MVP is explicitly designed so that adding Projects, Schedules, Contacts, and the full specificity rule system are additions to the rule schema (a mid-layer change) and do not require touching the conserved layer (pkg/contracts/notification.go, topic names, or the offline sync protocol).

---

## Appendix B — Key Go Dependencies

| Package | Purpose | Notes |
|---|---|---|
| `cloud.google.com/go/pubsub` | Pub/Sub client | Works against emulator automatically |
| `github.com/google/uuid` | UUID v7 generation | Or use `github.com/oklog/ulid` for sortable IDs |
| `modernc.org/sqlite` | SQLite — pure Go, no CGo | Easier cross-platform builds |
| `github.com/flyingmutant/rapid` | Property-based testing | Recommended for Go property tests |
| `net/http` | HTTP server — stdlib | No framework needed for MVP |

Avoid adding dependencies until a standard library option is clearly insufficient. Every dependency is conceptual mass.

---

*Plan created: 2026-05-31. Review and iterate before writing any code.*
