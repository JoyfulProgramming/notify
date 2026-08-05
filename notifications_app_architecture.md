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

7. A delivered notification persists across client disconnects.
   When a client reconnects, it receives all unread notifications delivered
   since the last connection, plus new live notifications.

8. Read status is per-location (e.g., web, Android app, desktop),
   but read-status changes broadcast to all locations for that user.
   When a notification is marked read on web, other locations are notified
   via a pub/sub event and hide it immediately from their UI.

9. Marking a notification as read is idempotent and irreversible.
   The operation sets a timestamp and is never undone.
   Every successful MarkRead publishes a notification-read event.

10. Read events are published to all locations.
    When any location marks a notification read, a notification-read event
    is published with user_id as a message attribute. All connected clients
    for that user receive and act on the event.
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

### Notification Delivery Status Schema — `notification-delivery.v1`

```json
{
  "user_id":           "string — owner of this notification",
  "location":          "string — where delivered (e.g., browser-web, app-android, desktop-macos)",
  "notification_id":   "string — UUID, references notification.v1",
  "delivered_at":      "string — ISO 8601, when first delivered to this user+location",
  "read_at":           "string or null — ISO 8601 when marked as read, null if unread",
  "source_app":        "string — copy of notification source_app for query performance",
  "title":             "string — copy of notification title",
  "body":              "string — copy of notification body",
  "metadata":          "object — arbitrary JSON, for extensibility"
}
```

**Contract (in plain language):**
- Every notification that passes the filter service is recorded here, keyed by `(user_id, location, notification_id)`.
- `read_at` is null until the user explicitly marks the notification as read. It is then set to the current timestamp.
- Storage is per-location, but UX is synchronized via events. When one device marks read, all connected clients immediately hide it.
- `delivered_at` is immutable and reflects when the notification first reached this user+location.
- Duplicate Record calls (same user_id, location, notification_id) are idempotent — they do not create duplicate rows.
- The `metadata` field stores raw `notification.v1` data as JSON for queryability and future evolution.

### Notification Read Event Schema — `notification-read.v1` (NEW)

```json
{
  "event_id":        "string — UUID v7, unique event identifier",
  "user_id":         "string — the user who marked it read",
  "notification_id": "string — UUID, which notification was marked read",
  "location":        "string — which location initiated the read (browser-web, app-android, etc)",
  "read_at":         "string — ISO 8601, when it was marked read"
}
```

**Contract (in plain language):**
- Emitted by notification-history when MarkRead is called.
- Published to a `notifications.read` topic with `user_id` as a Pub/Sub message attribute.
- Allows all other SSE connections for the same user to listen and immediately hide the notification from their UI.
- Provides cross-device read status synchronization without requiring global (database-level) read status.

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
| `delivery-service` | Subscribes to `notification-filtered`, delivers each notification to the user's connected clients via FCM/WebSocket/SSE, persists the delivery to `notification-history`, and serves the mark-read API. |
| `notification-history` | Persists all notifications delivered to each user by location, tracks read status by (user_id, location, notification_id), and provides queries for unread notifications. |
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

// A delivered notification is persisted and survives browser refresh.
func TestContract_DeliveredNotificationPersists(t *testing.T) {
    setUserRule(t, userID, Rule{SourceApp: "com.gmail", Action: DELIVER})
    id := publishNotification(t, Notification{SourceApp: "com.gmail", Title: "New email"})
    
    // Notification appears on first connection
    assertPresentInDeliveredStream(t, id, 5*time.Second)
    
    // Refresh the browser (disconnect and reconnect)
    disconnect(t)
    reconnect(t)
    
    // Unread notification still appears after reconnect
    assertPresentInUnreadNotifications(t, id, 5*time.Second)
}

// A notification marked as read is absent from the unread list.
func TestContract_MarkReadRemovesFromUnread(t *testing.T) {
    setUserRule(t, userID, Rule{SourceApp: "com.gmail", Action: DELIVER})
    id := publishNotification(t, Notification{SourceApp: "com.gmail", Title: "New email"})
    
    // Notification starts unread
    assertPresentInUnreadNotifications(t, id, 5*time.Second)
    
    // Mark it as read
    markNotificationRead(t, userID, id)
    
    // Now it's absent from unread list
    assertAbsentFromUnreadNotifications(t, id, 5*time.Second)
}

// Read status is per-location in storage, but events broadcast to connected clients.
func TestContract_ReadStatusPerLocation(t *testing.T) {
    setUserRule(t, userID, Rule{SourceApp: "com.gmail", Action: DELIVER})
    id := publishNotification(t, Notification{SourceApp: "com.gmail", Title: "New email"})
    
    // Deliver to both web and Android
    deliverToLocation(t, id, "browser-web")
    deliverToLocation(t, id, "app-android")
    
    // Mark as read on web only
    markNotificationReadOnLocation(t, userID, id, "browser-web")
    
    // Web: not in unread. Android: still in unread.
    assertAbsentFromUnreadNotifications(t, id, "browser-web")
    assertPresentInUnreadNotifications(t, id, "app-android")
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

// Marking a notification read is idempotent.
func TestProperty_MarkReadIdempotent(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        n := generateArbitraryNotification(t)
        rule := generateMatchingDeliverRule(t, n)
        setUserRule(t, userID, rule)
        id := publishNotification(t, n)
        
        // Mark as read twice
        markNotificationRead(t, userID, id)
        markNotificationRead(t, userID, id)  // idempotent
        
        // Should have zero or one read records, never duplicates
        readRecords := getReadRecords(t, userID, id)
        if len(readRecords) > 1 {
            t.Fatalf("idempotence violated: %d read records", len(readRecords))
        }
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
- **Unread notification count per user** — track for storage and performance planning
- **Read status lag** — from mark-read request to absence from unread list, must be < 1s
- **Persistence availability** — alert if notification history store has > 1 minute downtime

---

## Step 5 — Pace Layers (What Changes at What Speed)

```
SLOW LAYER — Almost Never Changes
  Pub/Sub message schema (notification.v1)
  Notification delivery status schema (notification-delivery.v1)
  Filter rule schema
  Read status semantics (per-location, idempotent, irreversible)
  Offline sync protocol (UUID-based dedup)
  Audit log format
  notification_id generation algorithm (UUID v7)

MID LAYER — Changes Monthly
  Filter Service: rule evaluation logic, priority resolution
  Rule API: CRUD operations, rule validation
  Delivery Service: channel routing, acknowledgement tracking, persistence calls
  Notification History Service: persistence, read status updates, unread queries

FAST LAYER — Changes Weekly or Daily
  Android notification listener (adapts to Android API changes)
  Web/Desktop UI: notification display, rule configuration, mark-read clicks  [UPDATED]
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
        ├──▶ notification-history (SQLite)
        │   ├──▶ Record(notification, location) — persist
        │   ├──▶ Unread(user_id, location) — query for web/app
        │   └──▶ MarkRead(user_id, location, notification_id) — update
        │
        └──▶ delivery-service
            ├──▶ Calls history.Record() after streaming
            ├──▶ Sends unread notifications from history.Unread() on connect
            ├──▶ Handles POST /notifications/:id/read via history.MarkRead()
            └──▶ Streams to WebSocket / SSE → Web / Desktop
            └──▶ Streams to FCM → Android
```

**Why it works:**
- The Filter Service is a pure function: `(notification, []Rule) → DELIVER | DISCARD`. It can be completely regenerated without touching Android or the web client — because the Pub/Sub schema is the conserved boundary.
- The ingestor is separable from the filter. You can regenerate one without the other.
- The delivery service is a thin adapter — it only knows about `notification-filtered` and delivery channels. New channels (e.g. Slack, email) are additions, not changes.
- **The notification-history service is a pure persistence layer.** It only knows about the `notification-delivery.v1` schema. The storage backend (SQLite, Postgres, DynamoDB) can be swapped without changing the service spec. Read status semantics are immutable, so queries remain consistent.

**Start here.** It is the smallest system that passes all the durable evaluations.

**With persistence (current requirement):** Add the notification-history service between the filter service and the delivery service. The history service is the authoritative store of delivered notifications. The delivery service consults it on reconnect to rebuild the unread list.

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
| `delivery-service` | `notification-filtered` schema, ack protocol, mark-read API | Delivery routing logic changes |
| `notification-history` | `notification-delivery.v1` schema, `notification-read.v1` events, read status semantics | Query/storage/event logic changes; schemas stay stable |
| `dead-letter-monitor` | Dead-letter topic contract | Alert logic changes |

---

## First Sprint — What to Build, In What Order

1. **Define and commit the Pub/Sub message schema.** This is the slow layer. It should be a Go struct in `pkg/contracts/notification.go` with a JSON schema alongside it. Do not write any services until this is done.

2. **Define the notification delivery status schema** (`notification-delivery.v1`). This is also a slow layer — once stable, services depend on it. Define the read status semantics: per-location, idempotent, irreversible.

3. **Write the durable evaluations.** Before any service code, write the contract tests and property tests above as Go test files that initially fail (they have nothing to test against yet). These become your acceptance criteria. Include persistence and read status tests.

4. **Build the Filter Service first.** It is a pure function — the easiest service to specify completely and the one everything else depends on for its output. Get it passing the contract tests.

5. **Build the ingestor.** Wire it to a local Pub/Sub emulator. Confirm the filter service receives notifications from it.

6. **Build the notification-history service.** Implement SQLite persistence: Record (idempotent persist), Unread (query by location), MarkRead (set read_at). Write and pass the persistence contract tests.

7. **Build the delivery service for one channel only** (WebSocket to a minimal web UI). Update it to: persist via notification-history after streaming, send unread notifications on connect, handle mark-read requests. Get end-to-end delivery and persistence working before adding FCM or SSE.

8. **Update the web frontend** to fetch and display unread notifications on page load, and add click handlers for mark-read.

9. **Add the Android listener last.** At this point the cloud pipeline is already tested. The Android code is a thin publisher — it only needs to produce valid `notification.v1` messages and implement the write-ahead log drain.

10. **Apply the n=1 test at week 4.** Could a new Go developer, given only the specs, invariants, and evaluations (not the code), regenerate the Filter Service? If not, improve the specs before writing more code.

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

### Notification Persistence & Read Status (NEW)

```
Why it exists:
  Users expect notifications to survive browser refresh or app restart.
  Without persistence, notifications vanish when a client disconnects.
  Read status must persist so users know what they've already seen.

Rejected alternatives:
  In-memory only: loses data on disconnect, requires rebuilding from Pub/Sub
  buffer (limited history, not user-facing).
  Global read status in database: would require complex sync logic and cause
  all devices to see read/unread in lockstep. Per-location storage + event
  broadcasting (Invariant 10) achieves the same UX with simpler schema.
  Forever retention: storage and query performance degrade. Cleanup after
  N days (e.g., 30) is acceptable.

Active assumptions:
  Users have at most ~10k unread notifications at a time.
  Read status queries (unread list) are common; archive queries are rare.
  SQLite is sufficient for a single-instance deployment.
  If scaling to distributed deployments, a shared database (PostgreSQL) is needed.

What would make this wrong:
  If users need retroactive read status changes (undo), add an undo_read
  timestamp alongside read_at.
  If users need to sync read status across all devices, add a global
  read_at column alongside the per-location one.
  If storage grows beyond SQLite's practical limits (100GB+), migrate to
  PostgreSQL or implement archival/tiering.
```

---

## Read Status Design: Per-Location vs. Global

The architecture uses **per-location read status** (Approach A). This section documents the decision and trade-offs.

### Approach A: Per-Location Read Status (Selected)

**Semantics:**
- Storage: each notification has a read_at timestamp per-location (browser-web, app-android, etc.).
- UX: when a notification is marked read on one location, a read event broadcasts to all other connected clients for that user, causing immediate hiding without refresh (Invariant 10).
- If a device reconnects later (was offline), it queries the per-location read_at and sees the correct state.
- Idempotent: marking the same notification read twice has no additional effect.
- Irreversible: once marked read, a notification cannot be unmarked.

**Schema:**
```sql
CREATE TABLE notifications_delivered (
  user_id TEXT NOT NULL,
  location TEXT NOT NULL,
  notification_id TEXT NOT NULL,
  delivered_at TEXT NOT NULL,
  read_at TEXT,  -- NULL = unread, ISO 8601 timestamp = read at that time
  source_app TEXT,
  title TEXT,
  body TEXT,
  metadata TEXT,
  
  UNIQUE(user_id, location, notification_id),
  INDEX(user_id, location, read_at, delivered_at)
);
```

**Queries:**
```sql
-- Unread notifications for a user+location, newest first
SELECT * FROM notifications_delivered
WHERE user_id = ? AND location = ? AND read_at IS NULL
ORDER BY delivered_at DESC;

-- Mark as read
UPDATE notifications_delivered
SET read_at = NOW()
WHERE user_id = ? AND location = ? AND notification_id = ?;
```

**Advantages:**
- Simple schema (one table, clear semantics).
- Real-time UX: read events broadcast to all connected devices, so marking read on phone hides it everywhere immediately (Invariant 10).
- Eventual consistency: devices that reconnect later (were offline) query the per-location read_at and see correct state.
- Easy to test: storage layer is per-location.
- Easy to scale to new locations: add support for `location='watch-os'` or `location='desktop-win'` without schema changes.
- Backend-agnostic: works with SQLite, PostgreSQL, DynamoDB, etc.

**Trade-offs:**
- Requires event-driven architecture (delivery-service must subscribe to notifications.read topic).
- If Pub/Sub is down, read events don't broadcast, but storage remains consistent (eventual sync on reconnect).

### Approach B: Global Read Status (Alternative, Not Selected)

**Semantics:**
- Marking a notification read on any device marks it read everywhere.
- One read_at timestamp per (user_id, notification_id) pair.
- Requires tracking which locations have delivered the notification (separate table).

**Schema:**
```sql
CREATE TABLE notifications (
  user_id TEXT NOT NULL,
  notification_id TEXT NOT NULL,
  first_delivered_at TEXT NOT NULL,
  read_at TEXT,  -- NULL = unread, ISO 8601 timestamp = read everywhere
  source_app TEXT,
  title TEXT,
  body TEXT,
  
  UNIQUE(user_id, notification_id),
  INDEX(user_id, read_at, first_delivered_at)
);

CREATE TABLE delivery_locations (
  user_id TEXT NOT NULL,
  notification_id TEXT NOT NULL,
  location TEXT NOT NULL,
  delivered_at TEXT NOT NULL,
  
  UNIQUE(user_id, notification_id, location),
  FOREIGN KEY(user_id, notification_id) REFERENCES notifications(user_id, notification_id)
);
```

**Advantages:**
- Mark once, read everywhere: simpler user experience.
- Fewer queries: one update touches all locations.
- Less storage: one read_at per notification, not per location.

**Disadvantages:**
- More complex schema (two tables, foreign keys).
- Reduces flexibility: with Approach B + event broadcasting, all devices are forced to show/hide in sync, which doesn't allow offline-first scenarios where a device might want independent read state until it reconnects.
- Harder to extend: if you later want per-location read status (e.g., for offline-first mobile apps), data migration is required.
- Couples the notification-history service's logic more tightly.

### Decision Rationale

**Per-Location (Approach A) was selected because:**
1. **Simpler schema** makes the notification-history service easier to specify, test, and regenerate.
2. **Matches user mental models** for multi-device workflows (phone, desktop, watch may be used independently).
3. **Forward-compatible** with future requirements (e.g., adding per-location notification preferences).
4. **Deletion-safe**: the schema and service can be rewritten with confidence that semantics are stable.

**If requirements change (e.g., users demand global read status):**
- Add a new `read_at` column to the `notifications` table (no deletion of the per-location column).
- Update the `MarkRead` logic to set both the per-location and global `read_at`.
- Query logic can then check global `read_at` first (for older users/browsers), falling back to per-location.
- This is a backward-compatible change that adds capability without breaking existing functionality.

---

## Implementation Phases

### Phase 1: Core Persistence (Current)
- [ ] Define `notification-delivery.v1` schema
- [ ] Implement `notification-history` service with per-location read status
- [ ] Update `delivery-service` to persist and serve mark-read
- [ ] Update web frontend to fetch and display unread on load

### Phase 2: Scaling (Future)
- [ ] If unread counts grow large, add pagination to `Unread()` queries
- [ ] If storage grows large, implement notification archival (older than 30 days)
- [ ] If read latency becomes an issue, add read-status caching in the delivery service

### Phase 3: Enhanced Multi-Device Control (Optional, If Requested)

Note: Real-time cross-device sync is already provided by Invariant 10 (read events). These are refinements:
- [ ] Add per-device read preferences (e.g., "stay unread on this device even if read elsewhere")
- [ ] Add global read status alongside per-location (dual-write strategy) if users demand true "all-or-nothing" read state
- [ ] Implement read-status reconciliation for offline devices that accumulated local read state

