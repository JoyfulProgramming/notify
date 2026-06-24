// Package deliver implements the Delivery bounded context's outward face
// (plan section 10): one SSE subscription per connected client, filtered to
// that client's user_id, deduplicated by notification id.
package deliver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"notify/internal/auth"
	"notify/pkg/bus"
	"notify/pkg/contracts"
)

type Service struct {
	bus bus.Bus
	mux *http.ServeMux
}

// New wires the SSE endpoint plus a static file server for web at "/".
func New(b bus.Bus, web fs.FS) *Service {
	s := &Service{bus: b}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	s.mux = mux
	return s
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Service) handleEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Subscribe before signalling 200 OK — a client must never be able to
	// observe a successful connection before its subscription exists, or a
	// notification published right after connecting could be missed.
	//
	// Per-connection filtered subscription — the local equivalent of a
	// Pub/Sub subscription with filter attributes.user_id="<id>" (plan
	// section 5, delivery-service fan-out model).
	sub := s.bus.Subscribe(bus.TopicNotificationsMatched, func(msg bus.Message) bool {
		return msg.Attributes["user_id"] == userID
	})
	defer sub.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	seen := make(map[string]bool) // INV-3: at most once per id, per session

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

		if seen[n.ID] {
			ack()
			continue
		}
		seen[n.ID] = true

		fmt.Fprintf(w, "data: %s\n\n", msg.Data)
		flusher.Flush()
		ack()
	}
}
