package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const requestLimit = 1 << 20

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	behavior := NewBehavior(cfg)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           routes(cfg, behavior),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopListening()

	go func() {
		<-ctx.Done()
		log.Printf("%s: signal received, shutting down", cfg.Name)
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("%s: forced shutdown: %v", cfg.Name, err)
		}
	}()

	log.Printf("%s listening on %s | %s", cfg.Name, server.Addr, cfg.Summary())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}

func routes(cfg Config, behavior *Behavior) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /quotes", quoteHandler(cfg, behavior))
	mux.HandleFunc("GET /healthz", healthHandler(cfg))
	mux.HandleFunc("GET /config", configHandler(cfg))
	return mux
}

func quoteHandler(cfg Config, behavior *Behavior) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decision := behavior.Admit()
		defer behavior.Complete()

		request, err := io.ReadAll(http.MaxBytesReader(w, r.Body, requestLimit))
		if err != nil {
			respond(w, cfg, decision, http.StatusRequestEntityTooLarge, jsonError{
				Error:   "request exceeds the limit accepted by the partner",
				Partner: cfg.Name,
			})
			return
		}

		if err := sleep(r.Context(), decision.Latency); err != nil {
			return
		}

		if decision.Fail {
			respond(w, cfg, decision, cfg.FailureStatus, jsonError{
				Error:   "partner unavailable",
				Partner: cfg.Name,
			})
			return
		}

		respond(w, cfg, decision, http.StatusOK, behavior.Quote(request))
	}
}

func healthHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "partner": cfg.Name})
	}
}

func configHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, cfg)
	}
}

type jsonError struct {
	Error   string `json:"error"`
	Partner string `json:"partner"`
}

func respond(w http.ResponseWriter, cfg Config, decision Decision, status int, body any) {
	header := w.Header()
	header.Set("X-Partner-Name", cfg.Name)
	header.Set("X-Partner-Seq", strconv.FormatUint(decision.Sequence, 10))
	header.Set("X-Partner-Latency-Ms", strconv.FormatInt(decision.Latency.Milliseconds(), 10))
	header.Set("X-Partner-Inflight", strconv.FormatInt(decision.InFlight, 10))
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
