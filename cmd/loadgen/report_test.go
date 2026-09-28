package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

func phaseOf(name string, concurrency int, elapsed time.Duration, results ...Result) Phase {
	started := time.Date(2026, 7, 30, 10, 39, 20, 0, time.UTC)
	return Phase{
		Name: name, Planned: len(results), Concurrency: concurrency,
		Started: started, Ended: started.Add(elapsed), Results: results,
	}
}

func ok(latency time.Duration) Result {
	return Result{Status: http.StatusOK, Latency: latency}
}

func badGateway(latency time.Duration, partner string) Result {
	return Result{Status: http.StatusBadGateway, Latency: latency, Partner: partner}
}

func TestSummaryCountsSuccessesAndTimesEveryRequest(t *testing.T) {
	phase := phaseOf("load", 4, 2*time.Second,
		ok(1*time.Second), ok(2*time.Second), badGateway(3*time.Second, "partner-flaky"), ok(4*time.Second))

	summary := summarize(phase)

	if summary.Requests != 4 || summary.OK != 3 {
		t.Fatalf("%d requests and %d successes, expected 4 and 3", summary.Requests, summary.OK)
	}
	if summary.SuccessRate() != 75 {
		t.Errorf("success rate %.0f%%, expected 75%%", summary.SuccessRate())
	}
	if summary.Max != 4*time.Second || summary.P50 != 2*time.Second {
		t.Errorf("max %s and p50 %s, expected 4s and 2s", summary.Max, summary.P50)
	}
	if summary.Throughput != 2 {
		t.Errorf("throughput %.1f req/s, expected 4 requests in 2s", summary.Throughput)
	}
}

func TestFailuresAreGroupedByCauseAndOrderedByWeight(t *testing.T) {
	phase := phaseOf("load", 4, time.Second,
		ok(time.Second),
		badGateway(time.Second, "partner-flaky"),
		badGateway(time.Second, "partner-flaky"),
		badGateway(time.Second, "partner-slow"),
		Result{Latency: time.Second, Failure: "timeout"},
	)

	causes := summarize(phase).Causes

	want := []Cause{
		{Label: "HTTP 502 from partner-flaky", Count: 2},
		{Label: "HTTP 502 from partner-slow", Count: 1},
		{Label: "timeout", Count: 1},
	}
	if len(causes) != len(want) {
		t.Fatalf("causes %v, expected %v", causes, want)
	}
	for i, cause := range causes {
		if cause != want[i] {
			t.Errorf("cause %d is %v, expected %v", i, cause, want[i])
		}
	}
}

func TestPercentileNeverInterpolates(t *testing.T) {
	sorted := make([]time.Duration, 0, 10)
	for i := 1; i <= 10; i++ {
		sorted = append(sorted, time.Duration(i)*time.Second)
	}

	cases := map[int]time.Duration{50: 5 * time.Second, 95: 10 * time.Second, 99: 10 * time.Second, 100: 10 * time.Second}
	for p, want := range cases {
		if got := percentile(sorted, p); got != want {
			t.Errorf("p%d = %s, expected %s", p, got, want)
		}
	}
	if got := percentile(nil, 95); got != 0 {
		t.Errorf("p95 of an empty phase = %s, expected zero", got)
	}
}

func TestReportPutsBaselineAndLoadSideBySide(t *testing.T) {
	baseline := phaseOf("baseline", 1, 9*time.Second, ok(2*time.Second), ok(2*time.Second))
	load := phaseOf("load", 50, 25*time.Second,
		ok(8*time.Second), badGateway(8*time.Second, "partner-flaky"))

	var out bytes.Buffer
	Report{
		Config:   Config{Target: "http://localhost:8080", Tenant: "corretora-a", Distinct: 5},
		Baseline: &baseline,
		Load:     load,
	}.Write(&out)

	report := out.String()
	for _, expected := range []string{
		"http://localhost:8080/quotes",
		"baseline — 2 requests, 1 in flight",
		"load — 2 requests, 50 in flight",
		"HTTP 502 from partner-flaky: 1",
		"p95 latency    2.00s → 8.00s   4.0x",
		"success        100% → 50%",
		"http://localhost:16686",
		"docs/roteiro-cenario-de-falha.md",
	} {
		if !strings.Contains(report, expected) {
			t.Errorf("the report does not show %q:\n%s", expected, report)
		}
	}
}

func TestReportWithoutBaselineDoesNotCompare(t *testing.T) {
	load := phaseOf("load", 50, 25*time.Second, ok(8*time.Second))

	var out bytes.Buffer
	Report{Config: Config{Target: "http://localhost:8080", Tenant: "corretora-a"}, Load: load}.Write(&out)

	if strings.Contains(out.String(), "baseline") {
		t.Errorf("the report compares against a baseline that was not measured:\n%s", out.String())
	}
}

func TestUnreachableEnvironmentIsNotAResult(t *testing.T) {
	refused := phaseOf("load", 4, time.Second,
		Result{Failure: "connection refused"}, Result{Failure: "connection refused"}, Result{Failure: "timeout"})
	answered := phaseOf("load", 4, time.Second, badGateway(time.Second, "partner-flaky"))

	reason, down := unreachable(nil, &refused)
	if !down {
		t.Error("a run with no answer at all should be reported as an environment that is down")
	}
	if reason != "connection refused" {
		t.Errorf("reason %q, expected the most frequent failure", reason)
	}
	if _, down := unreachable(nil, &answered); down {
		t.Error("the API answered 502: the environment is up and the run is valid")
	}
}

func TestDurationsAreReadableInTheReport(t *testing.T) {
	cases := map[time.Duration]string{
		120 * time.Millisecond:  "120ms",
		999 * time.Millisecond:  "999ms",
		1783 * time.Millisecond: "1.78s",
		8 * time.Second:         "8.00s",
	}
	for d, want := range cases {
		if got := short(d); got != want {
			t.Errorf("short(%s) = %q, expected %q", d, got, want)
		}
	}
}
