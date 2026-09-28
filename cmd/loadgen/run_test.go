package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/quotation"
)

func testConfig(target string) Config {
	return Config{
		Target: target, Tenant: "corretora-a",
		Concurrency: 4, Requests: 12, Baseline: 2,
		Duration: time.Minute, Timeout: 2 * time.Second, Distinct: 2,
	}
}

func testCaller(t *testing.T, cfg Config) *caller {
	t.Helper()
	bodies, err := quoteBodies(cfg.Distinct)
	if err != nil {
		t.Fatalf("quoteBodies: %v", err)
	}
	return newCaller(cfg, bodies)
}

type quotationAPI struct {
	*httptest.Server
	peak     atomic.Int64
	inFlight atomic.Int64
	served   atomic.Int64
}

func newQuotationAPI(latency time.Duration, handle http.HandlerFunc) *quotationAPI {
	api := &quotationAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.served.Add(1)
		current := api.inFlight.Add(1)
		for {
			peak := api.peak.Load()
			if current <= peak || api.peak.CompareAndSwap(peak, current) {
				break
			}
		}
		defer api.inFlight.Add(-1)

		select {
		case <-time.After(latency):
		case <-r.Context().Done():
			return
		}
		handle(w, r)
	}))
	return api
}

func quoteOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(quotation.Response{TenantID: "corretora-a", ElapsedMs: 1})
}

func TestLoadPhaseHoldsTheConfiguredNumberOfRequestsInFlight(t *testing.T) {
	api := newQuotationAPI(50*time.Millisecond, quoteOK)
	defer api.Close()

	cfg := testConfig(api.URL)
	phase := runPhase(context.Background(), testCaller(t, cfg), "load", cfg.Requests, cfg.Concurrency)

	if len(phase.Results) != cfg.Requests {
		t.Fatalf("%d results, expected the %d requests of the phase", len(phase.Results), cfg.Requests)
	}
	if peak := api.peak.Load(); peak != int64(cfg.Concurrency) {
		t.Errorf("peak of %d simultaneous requests, expected exactly %d", peak, cfg.Concurrency)
	}
	for _, result := range phase.Results {
		if !result.OK() || result.Latency <= 0 {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
}

func TestBaselinePhaseSendsOneRequestAtATime(t *testing.T) {
	api := newQuotationAPI(20*time.Millisecond, quoteOK)
	defer api.Close()

	cfg := testConfig(api.URL)
	phase := runPhase(context.Background(), testCaller(t, cfg), "baseline", cfg.Baseline, 1)

	if peak := api.peak.Load(); peak != 1 {
		t.Errorf("the baseline put %d requests in flight; without one at a time it is not a baseline", peak)
	}
	if len(phase.Results) != cfg.Baseline || phase.Planned != cfg.Baseline {
		t.Errorf("phase %+v does not match the %d planned requests", phase, cfg.Baseline)
	}
}

func TestFailureKeepsItsLatencyAndBlamesThePartner(t *testing.T) {
	api := newQuotationAPI(20*time.Millisecond, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"partner insurer unavailable","partner":"partner-flaky"}`)
	})
	defer api.Close()

	cfg := testConfig(api.URL)
	phase := runPhase(context.Background(), testCaller(t, cfg), "load", 4, 2)

	if len(phase.Results) != 4 {
		t.Fatalf("%d results, expected 4", len(phase.Results))
	}
	for _, result := range phase.Results {
		if result.OK() {
			t.Fatalf("the API answered 502 and the result came back as success: %+v", result)
		}
		if result.Partner != "partner-flaky" {
			t.Errorf("partner %q, expected the one the API blamed", result.Partner)
		}
		if result.Latency < 20*time.Millisecond {
			t.Errorf("latency %s, expected the time waited until the failure", result.Latency)
		}
		if got := cause(result); got != "HTTP 502 from partner-flaky" {
			t.Errorf("cause %q, expected the status alongside the partner", got)
		}
	}
}

func TestRequestBeyondTheTimeoutIsAFailureOfTheRun(t *testing.T) {
	api := newQuotationAPI(500*time.Millisecond, quoteOK)
	defer api.Close()

	cfg := testConfig(api.URL)
	cfg.Timeout = 50 * time.Millisecond
	phase := runPhase(context.Background(), testCaller(t, cfg), "load", 2, 2)

	if len(phase.Results) != 2 {
		t.Fatalf("%d results, expected 2", len(phase.Results))
	}
	for _, result := range phase.Results {
		if result.Status != 0 || result.Failure != "timeout" {
			t.Fatalf("expected a timeout, got %+v", result)
		}
	}
}

func TestRequestsCutShortByTheEndOfTheRunAreNotCounted(t *testing.T) {
	api := newQuotationAPI(time.Second, quoteOK)
	defer api.Close()

	cfg := testConfig(api.URL)
	cfg.Timeout = 0
	ctx, expire := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer expire()

	phase := runPhase(ctx, testCaller(t, cfg), "load", 100, 4)

	if len(phase.Results) != 0 {
		t.Fatalf("%d results, expected none: no request had time to be answered", len(phase.Results))
	}
	if phase.Planned != 100 {
		t.Errorf("planned %d, expected the phase to remember what it set out to send", phase.Planned)
	}
	if served := api.served.Load(); served > 8 {
		t.Errorf("%d requests sent after the end of the run", served)
	}
}

func TestEveryRequestIdentifiesTheBrokerAndRotatesTheQuotes(t *testing.T) {
	var mutex sync.Mutex
	tenants := map[string]int{}
	bodies := map[string]int{}

	api := newQuotationAPI(0, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mutex.Lock()
		tenants[r.Header.Get(quotation.TenantHeader)]++
		bodies[string(body)]++
		mutex.Unlock()
		quoteOK(w, r)
	})
	defer api.Close()

	cfg := testConfig(api.URL)
	cfg.Distinct = 3
	runPhase(context.Background(), testCaller(t, cfg), "load", 9, 1)

	if tenants["corretora-a"] != 9 {
		t.Errorf("tenant header arrived on %d of 9 requests: %v", tenants["corretora-a"], tenants)
	}
	if len(bodies) != 3 {
		t.Errorf("%d distinct quotes, expected the 3 that were configured", len(bodies))
	}
	for body, count := range bodies {
		if count != 3 {
			t.Errorf("quote %s appeared %d times, expected a balanced rotation", body, count)
		}
	}
}

func TestGeneratedQuoteIsAValidRequest(t *testing.T) {
	bodies, err := quoteBodies(5)
	if err != nil {
		t.Fatalf("quoteBodies: %v", err)
	}

	for _, body := range bodies {
		var request quotation.Request
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			t.Fatalf("body %s is not a quotation.Request: %v", body, err)
		}
		if err := request.Normalize(); err != nil {
			t.Fatalf("body %s does not pass the API's validation: %v", body, err)
		}
	}
}
