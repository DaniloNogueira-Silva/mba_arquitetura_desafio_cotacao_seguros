package main

import (
	"fmt"
	"testing"
)

const requestsPerRun = 210

type breakerPolicy struct {
	name             string
	consecutive      int
	window           int
	ratio            float64
	cooldown         int
	successesToClose int
	opensWithin      int
}

func (p breakerPolicy) trips(consecutiveFailures int, recent []bool) bool {
	if p.consecutive > 0 && consecutiveFailures >= p.consecutive {
		return true
	}
	if p.window > 0 && len(recent) == p.window {
		failures := 0
		for _, failed := range recent {
			if failed {
				failures++
			}
		}
		if float64(failures)/float64(p.window) >= p.ratio {
			return true
		}
	}
	return false
}

type breakerState int

const (
	closed breakerState = iota
	open
	halfOpen
)

type breakerOutcome struct {
	openedAt       int
	callsUntilOpen int
	opens          int
	recoveries     int
	shortCircuited int
}

func simulate(cfg Config, p breakerPolicy, requests int) breakerOutcome {
	behavior := NewBehavior(cfg)
	state := closed
	consecutiveFailures, consecutiveSuccesses, rejected, calls := 0, 0, 0, 0
	recent := make([]bool, 0, p.window)

	var out breakerOutcome
	for request := 1; request <= requests; request++ {
		if state == open {
			rejected++
			out.shortCircuited++
			if rejected >= p.cooldown {
				state, consecutiveSuccesses = halfOpen, 0
			}
			continue
		}

		decision := behavior.Admit()
		behavior.Complete()
		calls++

		if state == halfOpen {
			switch {
			case decision.Fail:
				state, rejected = open, 0
				out.opens++
			default:
				consecutiveSuccesses++
				if consecutiveSuccesses >= p.successesToClose {
					state, consecutiveFailures, recent = closed, 0, recent[:0]
					out.recoveries++
				}
			}
			continue
		}

		if decision.Fail {
			consecutiveFailures++
		} else {
			consecutiveFailures = 0
		}
		recent = append(recent, decision.Fail)
		if p.window > 0 && len(recent) > p.window {
			recent = recent[1:]
		}

		if p.trips(consecutiveFailures, recent) {
			state, rejected = open, 0
			out.opens++
			if out.openedAt == 0 {
				out.openedAt, out.callsUntilOpen = request, calls
			}
		}
	}
	return out
}

func (o breakerOutcome) String() string {
	if o.openedAt == 0 {
		return "never opened"
	}
	return fmt.Sprintf("opened on request %d (%d partner calls), %d openings, %d recoveries, %d requests short-circuited",
		o.openedAt, o.callsUntilOpen, o.opens, o.recoveries, o.shortCircuited)
}

func policies() []breakerPolicy {
	return []breakerPolicy{
		{name: "3 consecutive failures", consecutive: 3, cooldown: 5, successesToClose: 2, opensWithin: 10},
		{name: "5 consecutive failures", consecutive: 5, cooldown: 5, successesToClose: 2, opensWithin: 45},
		{name: "50% failures over a window of 10", window: 10, ratio: 0.5, cooldown: 5, successesToClose: 2, opensWithin: 15},
		{name: "60% failures over a window of 20", window: 20, ratio: 0.6, cooldown: 10, successesToClose: 3, opensWithin: 60},
	}
}

func TestFeasibilityBreakersOpenOnTheDefaultProfile(t *testing.T) {
	for _, policy := range policies() {
		t.Run(policy.name, func(t *testing.T) {
			out := simulate(flakyProfile(), policy, requestsPerRun)
			t.Log(out)

			if out.openedAt == 0 {
				t.Fatalf("the circuit never opened in %d requests — the defaults of partner-flaky make the challenge unbuildable", requestsPerRun)
			}
			if out.openedAt > policy.opensWithin {
				t.Fatalf("the circuit opened only on request %d, expected by request %d at the latest", out.openedAt, policy.opensWithin)
			}
		})
	}
}

func TestFeasibilityBreakersCloseAgain(t *testing.T) {
	for _, policy := range policies() {
		t.Run(policy.name, func(t *testing.T) {
			out := simulate(flakyProfile(), policy, requestsPerRun)
			if out.recoveries == 0 {
				t.Fatalf("the circuit opened %d times and never closed again in %d requests: the half-open state is unreachable with these defaults", out.opens, requestsPerRun)
			}
			t.Logf("%s — the breaker flaps, which is the point: a 40%% partner neither dies nor recovers for good", out)
		})
	}
}

func TestFeasibilityBurstIsWhereTheDocsSayItIs(t *testing.T) {
	const (
		wantLongest = 9
		wantEndsAt  = 57
	)

	longest, endsAt, current := 0, 0, 0
	for i, failed := range failureSequence(flakyProfile(), requestsPerRun) {
		if !failed {
			current = 0
			continue
		}
		current++
		if current > longest {
			longest, endsAt = current, i+1
		}
	}

	if longest != wantLongest || endsAt != wantEndsAt {
		t.Fatalf("longest burst of %d failures ending at sequence %d; the docs say %d ending at %d — re-run the smoke test and update docs/smoke-test-factibilidade.md",
			longest, endsAt, wantLongest, wantEndsAt)
	}
	t.Logf("burst of %d consecutive failures at sequences %d-%d, inside a single `make reproduce`", longest, endsAt-longest+1, endsAt)
}
