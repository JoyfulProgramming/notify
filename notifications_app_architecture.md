# Notifications App — Architecture Design

> Applying Phoenix Architecture principles to a multi-platform notification filtering system in Go.
> See [phoenix_architecture_summary.md](./phoenix_architecture_summary.md) for the underlying principles.

---

## What We're Building

A multi-platform notification filtering system:
- **Android** — intercepts device notifications, sends to cloud
- **Web / Desktop** — receives and displays filtered notifications
- **Google Cloud Pub/Sub** — the central event bus (source of truth)
- **Offline queue** — Android-side buffer when disconnected
- **User-defined filtering** — rules about which notifications to surface

This is a near-perfect use case for Phoenix Architecture because the core asset is *behaviour* (which notifications matter, to whom, under what rules) — not any particular implementation of that behaviour. The system spans multiple platforms, so regenerating any single component must not break the whole. Pub/Sub is a natural conserved layer — a stable boundary everything else targets.

---

## Step 1 — Invariants (Write These Before Any Code)

These must hold across **any** implementation, in any language, across any regeneration cycle.

```
1. A notification that enters the system is never silently discarded.
   It is either delivered, explicitly rejected by a user rule, or present
   in the dead-letter queue.

2. User filtering rules are the single source of truth for what is surfaced.
   No implementation may bypass them.

3. A notification is delivered at most once per client per session.
   Duplicates are suppressed by notification_id.

4. Offline notifications, once reconnected, appear in the same relative order
   they occurred on the device. They are never reordered by the cloud.

5. A rule change takes effect for all future notifications immediately.
   It does not retroactively affect already-delivered notifications.

6. Deleting a rule never causes a notification that was already delivered
   to disappear from a client's history.
```

These are your durable evaluations in plain English. Every contract test and property test you write should map to one of these.

---

## Step 2 — The Conserved Layer (The Slow Layer — Almost Never Changes)

These are the boundaries that survive all code regenerations. They require the most upfront thought and the most caution when changing.

### Pub/Sub Message Schema — `notification.v1`

```json
{
  "notification_id":  "string — UUID v7 (time-ordered), assigned by the publishing device",
  "source_app":       "string — Android package name (e.g. com.whatsapp)",
  "title":            "string — notification title, may be empty",
  "body":             "string — notification body, may be empty",
  "device_id":        "string — UUID identifying the source device",
  "device_timestamp": "string — ISO 8601, when the notification appeared on device",
  "received_at":      "string — ISO 8601, when this system first ingested it",
  "metadata":         "object — arbitrary string key-value pairs"
}
```

**Contract (in plain language):**
- `notification_id` is the deduplication key for the entire system. It is assigned once by the device and never changes.
- `device_timestamp` is what the device saw. `received_at` is when the system got it. Both are always present.
- No service may fail if an unknown `metadata` key is present.
- Adding new optional fields is a backwards-compatible change. Removing or renaming fields requires a new schema version (`notification.v2`).

### Filter Rule Schema

```json
{
  "rule_id":      "string — UUID, stable identifier for this rule",
  "user_id":      "string — owner of the rule",
  "source_app":   "string — package name to match, or '*' for any",
  "title_contains": "string — substring match on title, or '' for any",
  "action":       "string — DELIVER or DISCARD",
  "priority":     "integer — higher number wins when multiple rules match",
  "enabled":      "boolean"
}
```

### Offline Sync Protocol

The invariant (part of the conserved layer, never changes):

> Every notification captured by the Android listener is assigned a `notification_id` (UUID v7) on the device before any network attempt. The Android SQLite queue is a write-ahead log. On reconnect, the queue drains to Pub/Sub in chronological order. The cloud deduplicates by `notification_id`. Pub/Sub is always the source of truth; the device queue is always a delivery buffer.

This means offline reconciliation is **not** a two-way merge, not a conflict resolution problem, and not complex. It is a write-ahead log draining into an append-only event stream — the same pattern as how Kafka producers handle offline buffering.

---

## Step 3 — Component Specs (One Sentence Each)

Every component must be expressible in a single sentence that a developer with no other context could implement from:

| Component | One-sentence spec |
|---|---|
| `notification-ingestor` | Accepts raw notifications from Android/Web clients and publishes them to `notification-raw` Pub/Sub topic with a `received_at` timestamp, deduplicating by `notification_id`. |
| `filter-service` | Subscribes to `notification-raw`, evaluates each notification against the owner's active rules in priority order, and publishes matching notifications to `notification-filtered`. |
| `rule-api` | Provides CRUD operations for a user's filter rules and emits a `rule-changed` event to `rule-events` on every mutation. |
| `delivery-service` | Subscribes to `notification-filtered`, delivers each notification to the user's connected clients via FCM/WebSocket/SSE, and records acknowledgement by `notification_id`. |
| `notification-history` | Maintains an append-only read model of all notifications delivered to each user, queryable by time range and source app. |
| `dead-letter-monitor` | Consumes from the Pub/Sub dead-letter topic and emits alerts when undeliverable notifications accumulate beyond a threshold. |

If you cannot explain what a component does in one sentence, either the spec is unclear or the component is doing too much.

---

## Step 4 — Durable Evaluations

These tests survive any reimplementation. They speak only in terms of public contracts, not internal function names or data structures.

### Contract Tests (Black-Box, Against Deployed Services)

```go
// A notification matching an active DELIVER rule is present in the filtered stream.
func TestContract_MatchingRuleLeadsToDelivery(t *testing.T) {
    setUserRule(t, userID, Rule{SourceApp: "com.whatsapp", Action: DELIVER})
    id := publishNotification(t, Notification{SourceApp: "com.whatsapp", Title: "Alice: hey"})
    assertPresentInFilteredStream(t, id, 5*time.Second)
}

// A notification matching an active DISCARD rule is absent from the filtered stream.
func TestContract_DiscardRuleBlocksDelivery(t *testing.T) {
    setUserRule(t, userID, Rule{SourceApp: "com.twitter", Action: DISCARD})
    id := publishNotification(t, Notification{SourceApp: "com.twitter", Title: "Someone liked your tweet"})
    assertAbsentFromFilteredStream(t, id, 5*time.Second)
    assertPresentInDeadLetterOrAudit(t, id) // must appear somewhere — never silently lost
}

// Notifications sent while offline appear after reconnect, in order, no duplicates.
func TestContract_OfflineQueueDrainsInOrder(t *testing.T) {
    ids := sendNotificationsWhileOffline(t, 10)
    reconnect(t)
    assertDeliveredInOrder(t, ids, 15*time.Second)
    assertNoDuplicates(t, ids)
}
```

### Property Tests (Behavioural, Generated Inputs)

```go
// No notification matching an active DELIVER rule is ever silently absent.
func TestProperty_NoSilentDiscard(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        rule := generateMatchingDeliverRule(t, n)
        setUserRule(t, userID, rule)

        id := publishNotification(t, n)

        // Must appear in filtered stream OR audit log — never simply absent
        assertPresentInEitherStreamWithin(t, id, 10*time.Second)
    })
}

// Higher-priority rules always win when multiple rules match.
func TestProperty_PriorityOrdering(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        highPriority := Rule{SourceApp: n.SourceApp, Action: DELIVER, Priority: 100}
        lowPriority  := Rule{SourceApp: n.SourceApp, Action: DISCARD, Priority: 1}
        setUserRules(t, userID, highPriority, lowPriority)

        id := publishNotification(t, n)
        assertPresentInFilteredStream(t, id, 5*time.Second)
    })
}
```

### Invariant Monitoring (Live, Continuous)

These run in production continuously, not just in CI:

- **Delivery latency p99** — from `device_timestamp` to client acknowledgement, must be < 5s under normal load
- **Dead-letter accumulation rate** — alert if > 0.1% of notifications land in dead-letter
- **Filter rule hit rate** — track per-rule to detect stale rules that never match (compaction signal)
- **Offline queue depth** — per device, alert if queue > 1000 items (possible connectivity failure)
- **Duplicate delivery rate** — should be zero; any duplicate is a dedup invariant violation

---

## Step 5 — Pace Layers (What Changes at What Speed)

```
SLOW LAYER — Almost Never Changes
  Pub/Sub message schema (notification.v1)
  Filter rule schema
  Offline sync protocol (UUID-based dedup)
  Audit log format
  notification_id generation algorithm (UUID v7)

MID LAYER — Changes Monthly
  Filter Service: rule evaluation logic, priority resolution
  Rule API: CRUD operations, rule validation
  Delivery Service: channel routing, acknowledgement tracking
  Notification History: read model queries

FAST LAYER — Changes Weekly or Daily
  Android notification listener (adapts to Android API changes)
  Web/Desktop UI: notification display, rule configuration screens
  Push delivery adapters: FCM, WebSocket, SSE implementations
  Notification enrichment: grouping, summarisation, metadata tagging
```

The slow layer is encoded in the conserved schemas above. The mid layer is a set of Go services with clear specs. The fast layer is thin, platform-specific, and designed to be deleted and replaced as Android APIs evolve and the UI is refined.

---

## Three Architecture Approaches

---

### Architecture 1 — The Deletion-Safe Grain (Recommended Starting Point)

**Philosophy:** Every component is independently deletable and rewritable in a day.

```
Android App
  ├── Notification Listener (fast layer)
  ├── SQLite Queue (offline write-ahead log)
  └── Pub/Sub Publisher (reconnects, drains queue)
        │
        ▼
  notification-ingestor (Go)
        │
        ▼
  Google Cloud Pub/Sub
  topic: notification-raw
        │
        ▼
  filter-service (Go, stateless)
  reads from: rule-store (Postgres)
        │
        ▼
  Google Cloud Pub/Sub
  topic: notification-filtered
        │
        ├──▶ delivery-service → WebSocket / SSE → Web / Desktop
        ├──▶ delivery-service → FCM → Android
        └──▶ notification-history (append-only read model)
```

**Why it works:**
- The Filter Service is a pure function: `(notification, []Rule) → DELIVER | DISCARD`. It can be completely regenerated without touching Android or the web client — because the Pub/Sub schema is the conserved boundary.
- The ingestor is separable from the filter. You can regenerate one without the other.
- The delivery service is a thin adapter — it only knows about `notification-filtered` and delivery channels. New channels (e.g. Slack, email) are additions, not changes.

**Start here.** It is the smallest system that passes all the durable evaluations.

---

### Architecture 2 — Specification-Driven Filtering (Maximum Evolvability)

**Philosophy:** The filtering rules *are* the specification. Code is generated from them. The rule engine is the conserved layer.

```
Android / Web / Desktop
        │
        ▼
  notification-ingestor
        │
        ▼
  Google Cloud Pub/Sub (notification-raw)
        │
        ▼
  rule-evaluation-service (Go)
  ├── Rules stored as structured data in Postgres (not code)
  ├── Rule changes stored as events (full audit trail)
  └── Re-evaluates buffered recent events when rules change
        │
        ▼
  Per-user Pub/Sub subscriptions (filtered by user_id attribute)
        │
        ├──▶ FCM push (Android)
        ├──▶ SSE (Web)
        └──▶ WebSocket (Desktop)
```

**The key difference from Architecture 1:** rules are data with a formal schema. The rule evaluation engine is thin and can be regenerated freely. The rules themselves are the conserved layer — they represent the user's intent and survive any engine rewrite.

**Additional capability:** when a user changes a rule, the system can re-evaluate the last N minutes of buffered notifications against the new rule and surface anything they would have missed. The engine is stateless; the re-evaluation is just replaying Pub/Sub.

**Trade-off:** more upfront investment in the rule schema design, since it's the conserved layer. But once stable, the evaluation engine can be thrown away and regenerated in an afternoon.

**Offline reconciliation in this model:** Android holds a local snapshot of the user's current rules. When offline, it applies them locally to decide which notifications to buffer vs. discard locally. On reconnect, it sends the raw notification log to Pub/Sub for canonical re-evaluation. The device is an optimistic cache; the cloud is always authoritative.

---

### Architecture 3 — Phoenix Layers (Maximum Durability Under Change)

**Philosophy:** Explicitly separate the system into pace layers. Each layer regenerates at its natural speed. Encode the layers architecturally — not as convention, but as separate Go modules with enforced interfaces.

```
SLOW LAYER  ─────────────────────────────────────────────────
  Pub/Sub schemas (notification.v1, rule.v1)
  UUID v7 generation and dedup protocol
  Audit log (append-only, never mutated)
  pkg/contracts — Go types shared across all services

MID LAYER  ──────────────────────────────────────────────────
  cmd/filter-service    — rule evaluation, routing
  cmd/rule-api          — rule CRUD and rule-changed events
  cmd/delivery-service  — channel routing, acknowledgement
  cmd/notification-history — read model

FAST LAYER  ──────────────────────────────────────────────────
  android/              — Kotlin notification listener
  web/                  — React/HTMX notification UI
  cmd/delivery-fcm      — FCM adapter
  cmd/delivery-sse      — SSE adapter
  cmd/delivery-ws       — WebSocket adapter
```

**Slow layer is the only shared dependency.** Mid and fast layer services import from `pkg/contracts` for the shared types (the message schema structs), but nothing else. This enforces the pace layer separation at compile time.

**Offline reconciliation is a slow-layer invariant, not a service.** It's a protocol encoded in `pkg/contracts` as the dedup rules and queue drain algorithm. Any implementation of the Android publisher must conform to it. The cloud-side dedup is a property of the ingestor, not a separate service.

**When to choose this:** when you expect the Android platform to change significantly (Android API deprecations, new notification channels), and you want to ensure those changes are fully isolated in the fast layer. The slow layer types act as a firewall — fast layer code can be completely replaced without touching the mid or slow layers.

---

## Recommended Go Service Boundaries

Each passes the deletion test — it can be regenerated from its one-sentence spec:

| Service | Conserved boundary | Deletable if |
|---|---|---|
| `notification-ingestor` | `notification.v1` Pub/Sub schema | Ingestor logic changes; schema stays stable |
| `filter-service` | Rule evaluation contract, `notification-filtered` schema | Evaluation algorithm changes; contract stays stable |
| `rule-api` | Rule schema, `rule-changed` event shape | CRUD logic changes; schema stays stable |
| `delivery-service` | `notification-filtered` schema, ack protocol | Delivery routing logic changes |
| `notification-history` | Audit log schema | Query model changes; log schema stays stable |
| `dead-letter-monitor` | Dead-letter topic contract | Alert logic changes |

---

## First Sprint — What to Build, In What Order

1. **Define and commit the Pub/Sub message schema.** This is the slow layer. It should be a Go struct in `pkg/contracts/notification.go` with a JSON schema alongside it. Do not write any services until this is done.

2. **Write the durable evaluations.** Before any service code, write the contract tests and property tests above as Go test files that initially fail (they have nothing to test against yet). These become your acceptance criteria.

3. **Build the Filter Service first.** It is a pure function — the easiest service to specify completely and the one everything else depends on for its output. Get it passing the contract tests.

4. **Build the ingestor.** Wire it to a local Pub/Sub emulator. Confirm the filter service receives notifications from it.

5. **Build the delivery service for one channel only** (WebSocket to a minimal web UI). Get end-to-end delivery working before adding FCM or SSE.

6. **Add the Android listener last.** At this point the cloud pipeline is already tested. The Android code is a thin publisher — it only needs to produce valid `notification.v1` messages and implement the write-ahead log drain.

7. **Apply the n=1 test at week 4.** Could a new Go developer, given only the specs, invariants, and evaluations (not the code), regenerate the Filter Service? If not, improve the specs before writing more code.

---

## Provenance Records to Write Before Coding

Before generating any service, record:

### Filter Service

```
Why it exists:
  Filtering must happen in the cloud so rule changes apply to all devices
  simultaneously and the audit log is centralised.

Rejected alternatives:
  On-device filtering: rules would diverge between devices; a rule change on
  web wouldn't affect Android until next sync.
  Filter inside the ingestor: couples filtering to ingestion, prevents replay
  of historical events against new rules.

Active assumptions:
  Users have at most ~50 active rules.
  Rule evaluation is stateless (no rule references another rule's output).
  Pub/Sub delivers at least once; we handle deduplication.

What would make this wrong:
  If users need rule changes applied retroactively to historical events,
  a replay capability is needed.
  If rule count grows to thousands, the evaluation strategy needs rethinking.
```

### Offline Queue Protocol

```
Why it exists:
  Android devices lose connectivity. Notifications captured while offline
  must not be lost.

Rejected alternatives:
  Discard offline notifications: unacceptable data loss.
  Two-way sync on reconnect: introduces conflict resolution complexity that
  is unnecessary. The device only writes; the cloud is the source of truth.

Active assumptions:
  Device clock may drift; device_timestamp is informational, not authoritative
  for ordering. The cloud uses received_at for canonical ordering.
  notification_id (UUID v7) is unique per notification globally.

What would make this wrong:
  If devices need to receive notifications while offline (not just send them),
  a different offline model is needed (e.g. local rule evaluation with FCM
  high-priority messages).
```
