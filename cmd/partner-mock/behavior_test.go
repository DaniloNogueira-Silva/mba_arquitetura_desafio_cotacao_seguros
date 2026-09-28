package main

import (
	"testing"
	"time"
)

func flakyProfile() Config {
	return Config{
		Name:          "partner-flaky",
		Port:          "8080",
		Seed:          20260729,
		Latency:       150 * time.Millisecond,
		Jitter:        50 * time.Millisecond,
		FailureRate:   0.4,
		FailureStatus: 503,
		QuoteTTL:      300 * time.Second,
	}
}

func failureSequence(cfg Config, n int) []bool {
	b := NewBehavior(cfg)
	failures := make([]bool, n)
	for i := range failures {
		d := b.Admit()
		b.Complete()
		failures[i] = d.Fail
	}
	return failures
}

func TestFailuresAreIdenticalAcrossRuns(t *testing.T) {
	first := failureSequence(flakyProfile(), 500)
	second := failureSequence(flakyProfile(), 500)

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("request %d diverged between runs: %v and %v", i+1, first[i], second[i])
		}
	}
}

func TestDifferentSeedChangesTheSequence(t *testing.T) {
	cfg := flakyProfile()
	standard := failureSequence(cfg, 200)
	cfg.Seed = 1
	other := failureSequence(cfg, 200)

	for i := range standard {
		if standard[i] != other[i] {
			return
		}
	}
	t.Fatal("different seeds produced the same failure sequence")
}

func TestObservedFailureRateStaysCloseToTheConfiguredOne(t *testing.T) {
	const samples = 20000
	failures := 0
	for _, f := range failureSequence(flakyProfile(), samples) {
		if f {
			failures++
		}
	}

	observed := float64(failures) / samples
	if difference := observed - flakyProfile().FailureRate; difference > 0.02 || difference < -0.02 {
		t.Fatalf("observed failure rate %.4f, expected %.2f (tolerance 0.02)", observed, flakyProfile().FailureRate)
	}
}

func TestFlakyProfileProducesBursts(t *testing.T) {
	const window = 200
	const minimumBurst = 4

	longest, current := 0, 0
	for _, failed := range failureSequence(flakyProfile(), window) {
		if !failed {
			current = 0
			continue
		}
		current++
		if current > longest {
			longest = current
		}
	}

	if longest < minimumBurst {
		t.Fatalf("longest failure burst in the first %d requests was %d, expected at least %d", window, longest, minimumBurst)
	}
}

func TestRateZeroNeverFailsAndRateOneAlwaysFails(t *testing.T) {
	cfg := flakyProfile()
	cfg.FailureRate = 0
	for i, failed := range failureSequence(cfg, 100) {
		if failed {
			t.Fatalf("rate 0 failed on request %d", i+1)
		}
	}

	cfg.FailureRate = 1
	for i, failed := range failureSequence(cfg, 100) {
		if !failed {
			t.Fatalf("rate 1 succeeded on request %d", i+1)
		}
	}
}

func TestJitterStaysWithinTheRange(t *testing.T) {
	cfg := flakyProfile()
	b := NewBehavior(cfg)

	sawVariation := false
	for i := 0; i < 500; i++ {
		d := b.Admit()
		b.Complete()
		if d.Latency < cfg.Latency || d.Latency >= cfg.Latency+cfg.Jitter {
			t.Fatalf("latency %s outside the range [%s, %s)", d.Latency, cfg.Latency, cfg.Latency+cfg.Jitter)
		}
		if d.Latency != cfg.Latency {
			sawVariation = true
		}
	}
	if !sawVariation {
		t.Fatal("jitter configured but no latency varied")
	}
}

func TestDegradationOnlyStartsAboveTheThreshold(t *testing.T) {
	cfg := Config{
		Name:         "partner-degrading",
		Seed:         1,
		Latency:      120 * time.Millisecond,
		DegradeAfter: 5,
		DegradeStep:  300 * time.Millisecond,
		DegradeCap:   6 * time.Second,
		QuoteTTL:     300 * time.Second,
	}
	b := NewBehavior(cfg)

	cases := []struct {
		inFlight int64
		want     time.Duration
	}{
		{1, 0},
		{5, 0},
		{6, 300 * time.Millisecond},
		{10, 1500 * time.Millisecond},
		{25, 6 * time.Second},
		{100, 6 * time.Second},
	}

	for _, tc := range cases {
		if got := b.degradation(tc.inFlight); got != tc.want {
			t.Errorf("with %d in flight: degradation %s, expected %s", tc.inFlight, got, tc.want)
		}
	}
}

func TestDegradationIsOffWhenTheThresholdIsZero(t *testing.T) {
	b := NewBehavior(Config{Latency: 100 * time.Millisecond, DegradeStep: time.Second})
	if d := b.degradation(1000); d != 0 {
		t.Fatalf("degradation %s with PARTNER_DEGRADE_AFTER=0, expected 0", d)
	}
}

func TestAdmitCountsInFlightRequests(t *testing.T) {
	b := NewBehavior(flakyProfile())

	first := b.Admit()
	second := b.Admit()
	if first.InFlight != 1 || second.InFlight != 2 {
		t.Fatalf("in flight on arrival: %d and %d, expected 1 and 2", first.InFlight, second.InFlight)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequence: %d and %d, expected 1 and 2", first.Sequence, second.Sequence)
	}

	b.Complete()
	b.Complete()
	if remaining := b.InFlight(); remaining != 0 {
		t.Fatalf("%d requests in flight after completing all of them", remaining)
	}
}

func TestQuoteIsStablePerRequest(t *testing.T) {
	b := NewBehavior(flakyProfile())
	request := []byte(`{"tenant_id":"broker-a","driver_age":35}`)

	first := b.Quote(request)
	second := b.Quote(request)
	if first != second {
		t.Fatalf("the same request produced different quotes: %+v and %+v", first, second)
	}
	if other := b.Quote([]byte(`{"tenant_id":"broker-b","driver_age":35}`)); other == first {
		t.Fatal("different requests produced the same quote")
	}

	if first.Partner != "partner-flaky" {
		t.Errorf("partner %q, expected partner-flaky", first.Partner)
	}
	if first.Currency != "BRL" {
		t.Errorf("currency %q, expected BRL", first.Currency)
	}
	if first.PremiumCents < 50000 || first.PremiumCents > 200000 {
		t.Errorf("premium %d cents outside the expected range", first.PremiumCents)
	}
	if first.ValidForSeconds != 300 {
		t.Errorf("validity %d seconds, expected 300", first.ValidForSeconds)
	}
}

func TestDifferentPartnersQuoteDifferently(t *testing.T) {
	request := []byte(`{"tenant_id":"broker-a"}`)
	premiums := map[int64]string{}

	for _, name := range []string{"partner-slow", "partner-flaky", "partner-degrading"} {
		cfg := flakyProfile()
		cfg.Name = name
		quote := NewBehavior(cfg).Quote(request)
		if previous, repeated := premiums[quote.PremiumCents]; repeated {
			t.Fatalf("%s and %s quoted the same premium %d", previous, name, quote.PremiumCents)
		}
		premiums[quote.PremiumCents] = name
	}
}
