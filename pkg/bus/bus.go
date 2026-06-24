// Package bus is the local stand-in for the Google Cloud Pub/Sub boundary
// described in notify_mvp_plan.md section 5. It is intentionally public (not
// internal/) so the evaluations suite can attach raw subscribers to topics,
// the same way the original plan's tests would attach the Pub/Sub SDK
// directly to the emulator.
package bus

// Topic names — the conserved boundary (plan section 5).
const (
	TopicNotificationsCaptured           = "notifications.captured"
	TopicNotificationsMatched            = "notifications.matched"
	TopicNotificationsDiscarded          = "notifications.discarded"
	TopicRulesChanged                    = "rules.changed"
	TopicNotificationsCapturedDeadletter = "notifications.captured-deadletter"
)

// Message mirrors the shape of a real Pub/Sub message closely enough that
// swapping in a real GCP-backed Bus later is a contained change.
type Message struct {
	Data       []byte
	Attributes map[string]string
}

// Filter decides whether a subscription should receive a given message —
// the local equivalent of a Pub/Sub subscription filter expression
// (e.g. attributes.user_id = "abc123").
type Filter func(Message) bool

type AckFunc func()
type NackFunc func()

// Bus is the abstraction every service depends on instead of a concrete
// Pub/Sub client. internal/* packages only ever see this interface.
type Bus interface {
	Publish(topic string, msg Message) error
	Subscribe(topic string, filter Filter) *Subscription
}
