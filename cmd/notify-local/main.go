// notify-local is the entire Notify MVP pipeline in one process: ingestor,
// filter, rule-api, and delivery-service wired to an in-memory bus instead of
// a Pub/Sub emulator. No Docker, no other commands — just `go run ./cmd/notify-local`.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notify/internal/deliver"
	"notify/internal/filter"
	"notify/internal/ingestor"
	"notify/internal/rules"
	"notify/internal/rulestore"
	"notify/pkg/bus"
	"notify/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := rulestore.Open(envOr("NOTIFY_DB_PATH", "./notify.db"))
	if err != nil {
		log.Fatalf("opening rule store: %v", err)
	}
	defer store.Close()

	b := bus.NewMemory()

	filterSvc := filter.New(b, store)
	go func() {
		if err := filterSvc.Run(ctx); err != nil {
			log.Printf("filter-service stopped: %v", err)
		}
	}()

	servers := []*http.Server{
		{Addr: envOr("INGESTOR_ADDR", ":8080"), Handler: ingestor.New(b)},
		{Addr: envOr("RULES_ADDR", ":8081"), Handler: rules.New(b, store)},
		{Addr: envOr("DELIVER_ADDR", ":8082"), Handler: deliver.New(b, web.FS)},
	}

	for _, srv := range servers {
		srv := srv
		go func() {
			log.Printf("listening on %s", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server %s failed: %v", srv.Addr, err)
			}
		}()
	}

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		srv.Shutdown(shutdownCtx)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
