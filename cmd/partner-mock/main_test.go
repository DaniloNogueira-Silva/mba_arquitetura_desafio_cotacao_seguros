package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func requestQuote(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader(body))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	return response
}

func TestQuoteRespondsWithAQuoteWhenThePartnerIsHealthy(t *testing.T) {
	cfg := flakyProfile()
	cfg.Latency, cfg.Jitter, cfg.FailureRate = 0, 0, 0
	h := routes(cfg, NewBehavior(cfg))

	response := requestQuote(t, h, `{"tenant_id":"broker-a"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d, expected 200", response.Code)
	}

	var quote Quote
	if err := json.Unmarshal(response.Body.Bytes(), &quote); err != nil {
		t.Fatalf("response is not a quote: %v", err)
	}
	if quote.Partner != cfg.Name || quote.QuoteID == "" || quote.PremiumCents == 0 {
		t.Fatalf("incomplete quote: %+v", quote)
	}

	if got := response.Header().Get("X-Partner-Seq"); got != "1" {
		t.Errorf("X-Partner-Seq %q, expected 1", got)
	}
	if got := response.Header().Get("X-Partner-Name"); got != cfg.Name {
		t.Errorf("X-Partner-Name %q, expected %q", got, cfg.Name)
	}
	if got := response.Header().Get("X-Partner-Inflight"); got != "1" {
		t.Errorf("X-Partner-Inflight %q, expected 1", got)
	}
}

func TestQuoteFailsWithTheConfiguredStatus(t *testing.T) {
	cfg := flakyProfile()
	cfg.Latency, cfg.Jitter, cfg.FailureRate, cfg.FailureStatus = 0, 0, 1, 502
	h := routes(cfg, NewBehavior(cfg))

	response := requestQuote(t, h, `{}`)
	if response.Code != 502 {
		t.Fatalf("status %d, expected 502", response.Code)
	}

	var failure jsonError
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil {
		t.Fatalf("error response is not JSON: %v", err)
	}
	if failure.Partner != cfg.Name || failure.Error == "" {
		t.Fatalf("incomplete error: %+v", failure)
	}
}

func TestQuoteAppliesTheProfileLatency(t *testing.T) {
	cfg := flakyProfile()
	cfg.Latency, cfg.Jitter, cfg.FailureRate = 80*time.Millisecond, 0, 0
	h := routes(cfg, NewBehavior(cfg))

	start := time.Now()
	response := requestQuote(t, h, `{}`)
	elapsed := time.Since(start)

	if elapsed < cfg.Latency {
		t.Fatalf("response took %s, expected at least %s", elapsed, cfg.Latency)
	}
	if got := response.Header().Get("X-Partner-Latency-Ms"); got != "80" {
		t.Errorf("X-Partner-Latency-Ms %q, expected 80", got)
	}
}

func TestQuoteGivesUpWhenTheClientGivesUp(t *testing.T) {
	cfg := flakyProfile()
	cfg.Latency, cfg.Jitter, cfg.FailureRate = 5*time.Second, 0, 0
	behavior := NewBehavior(cfg)
	h := routes(cfg, behavior)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/quotes", strings.NewReader(`{}`)).WithContext(ctx)

	start := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), req)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("handler took %s after the client gave up", elapsed)
	}
	if inFlight := behavior.InFlight(); inFlight != 0 {
		t.Fatalf("%d requests in flight after the cancellation, expected 0", inFlight)
	}
}

func TestHealthzIgnoresTheProfile(t *testing.T) {
	cfg := flakyProfile()
	cfg.Latency, cfg.FailureRate = 5*time.Second, 1
	h := routes(cfg, NewBehavior(cfg))

	start := time.Now()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, expected 200", response.Code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("healthz took %s — the profile leaked into the healthcheck", elapsed)
	}
}

func TestConfigExposesTheEffectiveProfile(t *testing.T) {
	cfg := flakyProfile()
	h := routes(cfg, NewBehavior(cfg))

	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d, expected 200", response.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["partner"] != cfg.Name {
		t.Errorf("partner %v, expected %q", body["partner"], cfg.Name)
	}
	if body["failure_rate"] != cfg.FailureRate {
		t.Errorf("failure_rate %v, expected %v", body["failure_rate"], cfg.FailureRate)
	}
	if body["latency_ms"] != float64(cfg.Latency.Milliseconds()) {
		t.Errorf("latency_ms %v, expected %d", body["latency_ms"], cfg.Latency.Milliseconds())
	}
}
