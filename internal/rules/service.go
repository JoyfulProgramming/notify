// Package rules implements the Matching bounded context's rule-api (plan
// section 9): CRUD over a user's rules, with a RuleChangedEvent on every
// mutation. contracts.Rule carries no JSON tags by design (plan section 5 —
// serialisation is an adapter concern), so this package owns the wire DTO.
package rules

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"notify/internal/auth"
	"notify/internal/rulestore"
	"notify/pkg/bus"
	"notify/pkg/contracts"
)

type Service struct {
	bus   bus.Bus
	store *rulestore.Store
	mux   *http.ServeMux
}

func New(b bus.Bus, store *rulestore.Store) *Service {
	s := &Service{bus: b, store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rules", s.handleCreate)
	mux.HandleFunc("GET /rules", s.handleList)
	mux.HandleFunc("DELETE /rules/{id}", s.handleDelete)
	s.mux = mux
	return s
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type ruleDTO struct {
	ID            string `json:"id,omitempty"`
	SourceApp     string `json:"source_app"`
	SourceAccount string `json:"source_account"`
	Title         string `json:"title"`
}

func toDTO(r contracts.Rule) ruleDTO {
	return ruleDTO{ID: r.ID, SourceApp: r.SourceApp, SourceAccount: r.SourceAccount, Title: r.Title}
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var dto ruleDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "malformed body", http.StatusBadRequest)
		return
	}
	// A rule with every field empty is indistinguishable from a missing
	// request over HTTP — reject it here. Catch-all rules (deliberately all
	// fields empty) are still a valid domain concept; they're just not
	// reachable through this validated entry point in v1.
	if dto.SourceApp == "" && dto.SourceAccount == "" && dto.Title == "" {
		http.Error(w, "at least one field is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rule := contracts.Rule{
		ID:            id.String(),
		UserID:        userID,
		SourceApp:     dto.SourceApp,
		SourceAccount: dto.SourceAccount,
		Title:         dto.Title,
	}

	if err := s.store.Create(rule); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.publishChange(contracts.RuleCreated, rule)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toDTO(rule))
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rules, err := s.store.List(userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	dtos := make([]ruleDTO, 0, len(rules))
	for _, rule := range rules {
		dtos = append(dtos, toDTO(rule))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dtos)
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.FromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	existed, err := s.store.Delete(userID, id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !existed {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	s.publishChange(contracts.RuleDeleted, contracts.Rule{ID: id, UserID: userID})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) publishChange(kind contracts.RuleChangedKind, rule contracts.Rule) {
	eventID, err := uuid.NewV7()
	if err != nil {
		log.Printf("rules: failed to generate event id: %v", err)
		return
	}
	data, err := json.Marshal(contracts.RuleChangedEvent{
		EventID:   eventID.String(),
		Kind:      kind,
		Rule:      rule,
		ChangedAt: time.Now().UTC(),
	})
	if err != nil {
		log.Printf("rules: failed to marshal rule-changed event: %v", err)
		return
	}
	if err := s.bus.Publish(bus.TopicRulesChanged, bus.Message{
		Data:       data,
		Attributes: map[string]string{"user_id": rule.UserID},
	}); err != nil {
		log.Printf("rules: failed to publish rule-changed event: %v", err)
	}
}
