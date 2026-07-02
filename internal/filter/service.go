// Package filter implements the Matching bounded context's evaluation
// service (plan section 8): does this Capture-context Notification become a
// Delivery-context Notification? Answered by the user's rules alone (INV-2).
package filter

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"notify/internal/rulestore"
	"notify/pkg/bus"
	"notify/pkg/contracts"
)

type Service struct {
	bus   bus.Bus
	store *rulestore.Store
}

func New(b bus.Bus, store *rulestore.Store) *Service {
	return &Service{bus: b, store: store}
}

// Run subscribes to notifications.captured and routes every message to
// notifications.matched or notifications.discarded until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	sub := s.bus.Subscribe(bus.TopicNotificationsCaptured, nil)
	defer sub.Close()

	for {
		msg, ack, nack, err := sub.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, bus.ErrClosed) {
				return nil
			}
			return err
		}
		s.handle(msg, ack, nack)
	}
}

func (s *Service) handle(msg bus.Message, ack bus.AckFunc, nack bus.NackFunc) {
	var n contracts.Notification
	if err := json.Unmarshal(msg.Data, &n); err != nil {
		log.Printf("filter: dropping unparseable message: %v", err)
		ack()
		return
	}

	// Decision #4: an infrastructure failure must never be mistaken for a
	// rule decision. Nack so Pub/Sub (here: the in-memory bus) redelivers —
	// never route to notifications.discarded on a store error.
	rules, err := s.store.List(n.UserID())
	if err != nil {
		log.Printf("filter: rule store error, nacking for redelivery: %v", err)
		nack()
		return
	}

	topic := bus.TopicNotificationsDiscarded
	if rulestore.AnyRuleMatches(rules, n) {
		topic = bus.TopicNotificationsMatched
	}

	data, err := json.Marshal(n)
	if err != nil {
		log.Printf("filter: marshal error, dropping: %v", err)
		ack()
		return
	}

	if err := s.bus.Publish(topic, bus.Message{
		Data:       data,
		Attributes: map[string]string{"user_id": n.UserID()},
	}); err != nil {
		log.Printf("filter: publish error, nacking: %v", err)
		nack()
		return
	}

	ack()
}
