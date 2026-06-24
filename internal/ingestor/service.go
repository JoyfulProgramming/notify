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

	var n contracts.Notification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, "malformed body", http.StatusBadRequest)
		return
	}
	if n.SourceApp == "" {
		http.Error(w, "source_app is required", http.StatusBadRequest)
		return
	}

	// user_id is set here, from the authenticated caller, never trusted from
	// the request body — see plan section 5, Authentication Architecture.
	n.UserID = userID

	if n.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		n.ID = id.String()
	}
	if n.ReceivedAt.IsZero() {
		n.ReceivedAt = time.Now().UTC()
	}

	s.mu.Lock()
	duplicate := s.seen[n.ID]
	s.seen[n.ID] = true
	s.mu.Unlock()

	if !duplicate {
		data, err := json.Marshal(n)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		err = s.bus.Publish(bus.TopicNotificationsCaptured, bus.Message{
			Data:       data,
			Attributes: map[string]string{"user_id": n.UserID},
		})
		if err != nil {
			http.Error(w, "publish failed", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"id": n.ID})
}
