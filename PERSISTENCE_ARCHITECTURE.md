# Notification Persistence Architecture

## Requirement

Notifications should persist beyond browser refresh/reconnection. Users should only see notifications disappear when they explicitly mark them as read. Additionally, when a notification is marked as read on one location (e.g., web browser), all other locations for that user should be notified immediately so they can hide the notification from their UI without requiring a refresh.

This requires:
- Persisting delivered notifications to a durable store
- Loading historical (unread) notifications on client connect
- Providing a "mark as read" operation
- Ensuring read status persists across sessions
- Publishing read-status-changed events so all locations can synchronize in real-time

---

## Current State

**What works now:**
- Notifications flow through `pkg/bus` (in-memory)
- SSE streams them to browser
- Notifications disappear on refresh (no persistence)
- No notion of "read" status

**What must not change (Conserved Layer):**
- `notification.v1` Pub/Sub message schema
- Rule matching and filtering contracts
- The contract that a notification is delivered at most once per client per session

---

## Pace Layers: What Changes at What Speed?

### SLOW LAYER — Almost Never Changes

**Notification Delivery Status Schema**

```json
{
  "notification_id":  "UUID — identifies the notification",
  "user_id":          "string — the user who received it",
  "delivered_at":     "ISO 8601 — when it was first delivered to this user",
  "read_at":          "ISO 8601 or null — when user marked it read, or null if unread",
  "location":         "string — where it was delivered (browser-web, app-android, etc)",
  "source_app":       "string — original source app",
  "title":            "string — notification title",
  "body":             "string — notification body"
}
```

**Contracts (in plain English):**

1. Every notification that reaches the delivery layer is recorded in the notification store, keyed by (user_id, notification_id, location).
2. A notification is unread until the user explicitly marks it as read via the mark-read endpoint.
3. Marking a notification as read is idempotent — marking it twice has the same effect as once.
4. When a client reconnects, it receives all unread notifications, in delivered order (newest first or oldest first, configurable).
5. Read status is per-location in storage (web and Android each have independent read_at timestamps). However, when a notification is marked read on one location, a read event broadcasts to all connected clients for that user, causing immediate hiding (Invariant 10). If a device is offline when marked read elsewhere, it will see the correct read_at when reconnected.
6. Rule changes do not affect already-delivered notifications' read status or visibility.

### MID LAYER — Changes Monthly

**notification-history service** — Maintains append-only record of delivered notifications. Handles:
- Writing notification + delivery metadata to store
- Querying unread notifications for a user + location
- Marking notifications as read by (user_id, notification_id)
- Querying read history (optional, for "archive" or "all notifications" view)

**delivery service** — Updated to call notification-history after streaming.

### FAST LAYER — Changes Weekly

**Frontend** — Updated to:
- Fetch unread notifications on page load
- Render them alongside streamed notifications
- Call mark-read endpoint when user interacts
- Handle read status updates in real-time via SSE

---

## Three Implementation Approaches

---

### Approach A: Per-Location Read Status + Event Broadcasting (Recommended)

**Philosophy:** Read status is per-location in storage, but read events broadcast changes in real-time to all connected clients (Invariant 10). Storage stays simple while UX synchronizes immediately.

```
Delivery Service (8082)
  ├─▶ Stream notification to user via SSE (browser)
  └─▶ Call notification-history.Record()
        │
        ├─▶ SQLite: INSERT INTO notifications_delivered
        │           (user_id, location, notification_id, delivered_at, ...)
        │
        └─▶ Emit "notification-delivered" event (for audit/monitoring)

GET /notifications/unread (browser requests on load)
  └─▶ SELECT * FROM notifications_delivered
      WHERE user_id = ?
        AND location = 'browser-web'
        AND read_at IS NULL
      ORDER BY delivered_at DESC

POST /notifications/:id/read
  └─▶ UPDATE notifications_delivered
      SET read_at = NOW()
      WHERE user_id = ? AND notification_id = ? AND location = 'browser-web'
      └─▶ Emit "notification-read" event (SSE broadcast to all clients for that user)
```

**Schema:**

```sql
CREATE TABLE notifications_delivered (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  location TEXT NOT NULL,  -- 'browser-web', 'app-android', etc
  notification_id TEXT NOT NULL,
  delivered_at TEXT NOT NULL,  -- ISO 8601
  read_at TEXT,  -- ISO 8601, NULL = unread
  source_app TEXT,
  title TEXT,
  body TEXT,
  metadata TEXT,  -- JSON, for extensibility
  
  UNIQUE(user_id, location, notification_id),
  INDEX(user_id, location, read_at, delivered_at)
);
```

**Why this approach:**
- **Simple schema:** per-location read_at timestamp, no need to track devices or sync logic in storage.
- **Works with multiple clients:** web, Android, desktop queries return correct state for each.
- **Real-time UX via events:** read events broadcast immediately to all connected clients, so mark-read on phone hides it on all open browsers without extra clicks.
- **Eventual consistency:** if a device reconnects later (offline), it queries the store and sees the read_at timestamp — correct state without double-reads.
- **Deletable:** the notification-history service is a thin adapter — it's easy to rewrite the query logic or storage backend.

**Trade-off:** Storage layer is per-location, but events synchronize UX across locations. Requires event-driven architecture in delivery-service.

---

### Approach B: User-Level Read Status (Synchronised Across Locations)

**Philosophy:** Read status is global per user. Marking a notification read anywhere marks it read everywhere.

```
POST /notifications/:id/read
  └─▶ UPDATE notifications_delivered
      SET read_at = NOW()
      WHERE user_id = ? AND notification_id = ?  -- NO location filter
      └─▶ Emit "notification-read" event (broadcast to all of user's connected clients)
```

**Schema:**

```sql
CREATE TABLE notifications_delivered (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  notification_id TEXT NOT NULL,
  first_delivered_at TEXT NOT NULL,  -- earliest delivery across all locations
  read_at TEXT,  -- ISO 8601, NULL = unread
  source_app TEXT,
  title TEXT,
  body TEXT,
  
  UNIQUE(user_id, notification_id),
  INDEX(user_id, read_at, first_delivered_at)
);

CREATE TABLE delivery_locations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  notification_id TEXT NOT NULL,
  location TEXT NOT NULL,  -- 'browser-web', 'app-android', etc
  delivered_at TEXT NOT NULL,
  
  UNIQUE(user_id, notification_id, location),
  FOREIGN KEY(user_id, notification_id) 
    REFERENCES notifications_delivered(user_id, notification_id)
);
```

**Why this approach:**
- **Simpler for users:** mark read once, it's read everywhere.
- **Less notification fatigue:** you don't see the same notification twice.

**Trade-off:** More complex schema (two tables). The notification-history service needs more logic. If you want to track "saw on phone but not on web," you'd need additional columns.

---

### Approach C: Archive + Read (Hybrid)

**Philosophy:** By default, read status is per-location (like A). But users can optionally enable "sync read status across devices."

**Not recommended for first iteration.** Add if users demand it later.

---

## Recommended: Approach A (Per-Location) + Event Broadcasting (Invariant 10)

**Rationale:**
1. **Simpler schema** — one table with clear semantics. No need for complex sync logic in storage.
2. **Real-time UX** — read events (notifications.read topic) broadcast immediately to all connected clients, so marking read on one device hides it on all open browsers without requiring refresh.
3. **Eventual consistency** — if a device reconnects later (was offline), it queries the per-location read_at timestamp and sees the correct state, ensuring consistency even without event delivery.
4. **True multi-device support** — storage is per-location (each device can have independent read_at), but UX is synchronized (events hide notifications immediately across devices).
5. **Aligns with Phoenix Architecture** — the service is small and the schemas are stable; swapping storage backends or event implementations is easy.

---

## New Events & Pub/Sub Topics

### notification-read Event (NEW)

**Topic:** `notifications.read`

**Schema:**
```json
{
  "event_id":        "UUID v7 — unique event identifier",
  "user_id":         "string — the user who marked it read",
  "notification_id": "UUID — which notification was marked read",
  "location":        "string — which location initiated (browser-web, app-android, etc)",
  "read_at":         "ISO 8601 — when it was marked read"
}
```

**Pub/Sub Attributes:**
```
user_id = <user_id>  (for per-user subscription filtering)
```

**Publishing Contract:**
- Emitted when `notification-history.MarkRead()` succeeds
- Published once per unique (user_id, notification_id, location) pair
- Idempotent: if MarkRead is called multiple times with same args, only one event is published

**Subscription Model (in delivery-service):**
```
Topic: notifications.read
Filter: attributes.user_id = "<user_id>"
→ All connected SSE clients for that user receive the event
→ Client's browser removes the notification from display immediately
```

**Why this works:**
- No need for global read status in storage (stays per-location)
- All locations get real-time notification of read status changes
- Event-driven architecture is loosely coupled
- Each location can still have independent read status if needed (e.g., user opens Android phone later and queries the history service)

---

## New Components & Changes

### Updated: `notification-history` service

**One-sentence spec:**  
Persists every notification delivered to a user + location, records read status, publishes read-status-changed events, and provides queries for unread notifications by location.

**Exports (package `internal/history`):**

```go
type Service interface {
  // Record writes a delivered notification to the store.
  // Idempotent: duplicate calls with the same (user_id, location, notification_id)
  // are silently ignored (not errors).
  Record(ctx context.Context, userID, location string, n *contracts.Notification) error
  
  // MarkRead sets read_at = now() for the given notification.
  // Idempotent. Publishes a notification-read event on success.
  MarkRead(ctx context.Context, userID, location, notificationID string) error
  
  // Unread returns all unread notifications for user + location,
  // sorted by delivered_at DESC (newest first).
  Unread(ctx context.Context, userID, location string) ([]DeliveredNotification, error)
  
  // Cleanup (optional, for testing) — delete all records for a user.
  Cleanup(ctx context.Context, userID string) error
}

type DeliveredNotification struct {
  NotificationID string
  SourceApp      string
  Title          string
  Body           string
  DeliveredAt    time.Time
  ReadAt         *time.Time  // nil = unread
  Location       string
}
```

### Modified: `delivery-service`

**Changes to `internal/deliver/service.go`:**

1. Accept a `history.Service` in `New()`.
2. After streaming each notification via SSE, call `history.Record()` to persist it.
3. Add a new HTTP handler `POST /notifications/:id/read` that calls `history.MarkRead()`.
4. Subscribe to `notifications.read` topic (NEW) and broadcast read events to all connected SSE clients for that user.

**Code outline:**

```go
func (s *Service) handleEvents(w http.ResponseWriter, r *http.Request) {
  userID, ok := auth.FromRequest(r)
  if !ok {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
  }

  sub := s.bus.Subscribe(bus.TopicNotificationsMatched, func(msg bus.Message) bool {
    return msg.Attributes["user_id"] == userID
  })
  defer sub.Close()

  sse := datastar.NewSSE(w, r)
  sse.PatchSignals([]byte(`{"status":"connected"}`))
  
  // Send unread notifications on connect (NEW)
  unread, _ := s.history.Unread(r.Context(), userID, "browser-web")
  for _, n := range unread {
    s.sendNotificationFragment(sse, n.NotificationID, n.Title, n.Body, n.SourceApp)
  }

  ctx := r.Context()
  seen := make(map[string]bool)

  for {
    msg, ack, _, err := sub.Receive(ctx)
    if err != nil {
      return
    }

    var n contracts.Notification
    if err := json.Unmarshal(msg.Data, &n); err != nil {
      ack()
      continue
    }

    if seen[n.ID()] {
      ack()
      continue
    }
    seen[n.ID()] = true

    // Persist to store (NEW)
    _ = s.history.Record(ctx, userID, "browser-web", &n)

    // Send to browser (EXISTING)
    fragment := fmt.Sprintf(...)
    _ = sse.PatchElements(fragment, ...)
    ack()
  }
}

// New handler (NEW)
func (s *Service) handleMarkRead(w http.ResponseWriter, r *http.Request) {
  userID, ok := auth.FromRequest(r)
  if !ok {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
  }
  
  notificationID := r.PathValue("id")
  if err := s.history.MarkRead(r.Context(), userID, "browser-web", notificationID); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
  
  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(map[string]string{"status": "read"})
}
```

### New: Web/Frontend Changes

**On page load:**
```javascript
// Fetch unread notifications from server
fetch('/notifications/unread')
  .then(r => r.json())
  .then(notifications => {
    // Render each unread notification
    notifications.forEach(n => renderNotification(n));
  });

// Then subscribe to SSE for live updates (existing code)
const eventSource = new EventSource('/events');
```

**On clicking "mark as read":**
```javascript
function markRead(notificationId) {
  fetch(`/notifications/${notificationId}/read`, { method: 'POST' })
    .then(() => {
      // Remove from UI
      document.getElementById(`notification-${notificationId}`).remove();
    });
}
```

---

## New Durable Evaluations (Contracts)

These tests survive any reimplementation:

```go
// A delivered notification is present in the unread list.
func TestContract_DeliveredNotificationIsUnread(t *testing.T) {
  n := newTestNotification("com.google.gmail", "New message")
  n.SetID("test-123")
  
  // Simulate delivery
  historyService.Record(ctx, userID, "browser-web", n)
  
  unread, _ := historyService.Unread(ctx, userID, "browser-web")
  if !contains(unread, "test-123") {
    t.Fatal("delivered notification not in unread list")
  }
}

// A marked-as-read notification is absent from the unread list.
func TestContract_MarkedReadNotificationHidden(t *testing.T) {
  n := newTestNotification("com.google.gmail", "New message")
  n.SetID("test-123")
  
  historyService.Record(ctx, userID, "browser-web", n)
  historyService.MarkRead(ctx, userID, "browser-web", "test-123")
  
  unread, _ := historyService.Unread(ctx, userID, "browser-web")
  if contains(unread, "test-123") {
    t.Fatal("read notification still in unread list")
  }
}

// Read status is per-location in storage, but events broadcast to connected clients.
func TestContract_ReadStatusPerLocation(t *testing.T) {
  n := newTestNotification("com.google.gmail", "New message")
  n.SetID("test-123")
  
  historyService.Record(ctx, userID, "browser-web", n)
  historyService.Record(ctx, userID, "app-android", n)
  historyService.MarkRead(ctx, userID, "browser-web", "test-123")
  
  webUnread, _ := historyService.Unread(ctx, userID, "browser-web")
  androidUnread, _ := historyService.Unread(ctx, userID, "app-android")
  
  if contains(webUnread, "test-123") {
    t.Fatal("notification should be read on web")
  }
  if !contains(androidUnread, "test-123") {
    t.Fatal("notification should still be unread on Android")
  }
}

// Idempotence: marking a notification read twice is safe.
func TestContract_MarkReadIdempotent(t *testing.T) {
  n := newTestNotification("com.google.gmail", "New message")
  n.SetID("test-123")
  
  historyService.Record(ctx, userID, "browser-web", n)
  historyService.MarkRead(ctx, userID, "browser-web", "test-123")
  historyService.MarkRead(ctx, userID, "browser-web", "test-123")  // Call twice
  
  unread, _ := historyService.Unread(ctx, userID, "browser-web")
  if contains(unread, "test-123") {
    t.Fatal("idempotence violated")
  }
}

// Duplicate record calls are idempotent.
func TestContract_RecordIdempotent(t *testing.T) {
  n := newTestNotification("com.google.gmail", "New message")
  n.SetID("test-123")
  
  historyService.Record(ctx, userID, "browser-web", n)
  historyService.Record(ctx, userID, "browser-web", n)  // Call twice
  
  unread, _ := historyService.Unread(ctx, userID, "browser-web")
  count := countBy(unread, func(x DeliveredNotification) bool { return x.NotificationID == "test-123" })
  if count != 1 {
    t.Fatalf("duplicate record created %d rows", count)
  }
}
```

---

## Summary of Changes

| Component | Change | Deletable? |
|---|---|---|
| `internal/history` | **NEW** — Notification store + queries | Yes — schema stays stable |
| `internal/deliver` | **UPDATED** — Persist on delivery, add mark-read endpoint | Yes — contracts stay stable |
| `web/` | **UPDATED** — Load unread on page load, add mark-read clicks | Yes — API contract stays stable |
| `pkg/contracts` | **UNCHANGED** — No new contracts needed | N/A |
| `internal/filter` | **UNCHANGED** | N/A |
| `internal/ingestor` | **UNCHANGED** | N/A |
| `internal/rules` | **UNCHANGED** | N/A |

---

## Order of Implementation

1. **Define the `notification-history` service interface** in `pkg/contracts` or `internal/history/service.go`.
2. **Write the durable evaluations** (contract tests above) — they should initially fail.
3. **Implement the SQLite-backed history service** in `internal/history/store.go`.
4. **Update the delivery service** to call `history.Record()` and wire the new handler.
5. **Update the frontend** to fetch unread on load and call mark-read endpoints.
6. **Write integration tests** — end-to-end: deliver → persist → refresh → see notification → mark read → refresh → gone.

---

## Risk Mitigation

**Risk:** "What if the notification schema changes?"  
**Mitigation:** Store raw `contracts.Notification` as JSON in the `metadata` column. If the schema evolves, old records still have the full data.

**Risk:** "What if we want to switch to a different database?"  
**Mitigation:** The `history.Service` interface is the boundary. The SQLite implementation is an implementation detail inside `internal/history`. Swapping to Postgres means rewriting only `internal/history/store.go`.

**Risk:** "What if the user has thousands of notifications?"  
**Mitigation:** The schema includes an index on `(user_id, location, read_at, delivered_at)`. Queries for unread notifications are fast. If the table grows very large, a "delete notifications older than 30 days" cleanup can run as a background task.

---

## Backwards Compatibility

**No breaking changes.**
- The existing SSE contract is unchanged.
- The existing rule/filter contracts are unchanged.
- New endpoints (`GET /notifications/unread`, `POST /notifications/:id/read`) are additive.
- Existing clients that don't call the new endpoints work as before (they just won't see notifications after refresh).

---

## Non-Goals (For Later)

- **Per-notification TTL:** e.g. "delete this notification after 7 days." Can add as a column later.
- **Notification grouping:** e.g. "show 5 messages, then 'and 10 more'". Can be added to the frontend.
- **Full-text search:** e.g. "search my notification history." Not needed for MVP, add later.
- **Read status sync across devices:** Would add significant complexity. Per-location is simpler and works well.
