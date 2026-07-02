// Package ingestor implements the Capture bounded context's single entry
// point (plan section 7): authenticate, stamp identity, validate, publish.
package ingestor

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"notify/internal/auth"
	"notify/pkg/bus"
	"notify/pkg/contracts"
)

// rawNotification is the inbound POST /notifications request shape. It
// exists because the request legitimately omits fields the server fills in
// (id, user_id, device_timestamp) — decoding straight into a
// contracts.Notification would reject those as incomplete, since
// contracts.NewNotification is the only way to obtain one and requires them
// all up front. See internal/rules/service.go's ruleDTO for the same pattern.
type rawNotification struct {
	ID              string            `json:"id"`
	SourceApp       string            `json:"source_app"`
	SourceAccount   string            `json:"source_account"`
	SourceID        string            `json:"source_id"`
	SentBy          string            `json:"sent_by"`
	SentIn          string            `json:"sent_in"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	DeviceID        string            `json:"device_id"`
	DeviceTimestamp time.Time         `json:"device_timestamp"`
	Metadata        map[string]string `json:"metadata"`
}

type Service struct {
	bus  bus.Bus
	mux  *http.ServeMux
	mu   sync.Mutex
	seen map[string]bool // dedup by id — single-instance assumption, MVP scope
}

func New(b bus.Bus) *Service {
	s := &Service{bus: b, seen: make(map[string]bool)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /notifications", s.handlePost)
	s.mux = mux
	return s
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Service) handlePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var dto rawNotification
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "malformed body", http.StatusBadRequest)
		return
	}

	// id and received_at are filled in here if the client omitted them;
	// device_timestamp likewise falls back to received_at when the client
	// (or device) didn't report one — same "filled in if absent" treatment
	// as id, see specs/notification.json.
	id := dto.ID
	if id == "" {
		generated, err := uuid.NewV7()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		id = generated.String()
	}
	receivedAt := time.Now().UTC()
	deviceTimestamp := dto.DeviceTimestamp
	if deviceTimestamp.IsZero() {
		deviceTimestamp = receivedAt
	}

	// user_id is set here, from the authenticated caller, never trusted from
	// the request body — see plan section 5, Authentication Architecture.
	n, err := contracts.NewNotification(contracts.NotificationParams{
		ID:              id,
		UserID:          userID,
		SourceApp:       dto.SourceApp,
		SourceAccount:   dto.SourceAccount,
		SourceID:        dto.SourceID,
		SentBy:          dto.SentBy,
		SentIn:          dto.SentIn,
		Title:           dto.Title,
		Body:            dto.Body,
		DeviceID:        dto.DeviceID,
		DeviceTimestamp: deviceTimestamp,
		ReceivedAt:      receivedAt,
		Metadata:        dto.Metadata,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	duplicate := s.seen[n.ID()]
	s.seen[n.ID()] = true
	s.mu.Unlock()

	if !duplicate {
		data, err := json.Marshal(n)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		err = s.bus.Publish(bus.TopicNotificationsCaptured, bus.Message{
			Data:       data,
			Attributes: map[string]string{"user_id": n.UserID()},
		})
		if err != nil {
			http.Error(w, "publish failed", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"id": n.ID()})
}
