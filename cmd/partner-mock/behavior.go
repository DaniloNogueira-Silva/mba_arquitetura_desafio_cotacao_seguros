package main

import (
	"fmt"
	"hash/fnv"
	"sync/atomic"
	"time"
)

type Behavior struct {
	cfg      Config
	seq      atomic.Uint64
	inFlight atomic.Int64
}

func NewBehavior(cfg Config) *Behavior {
	return &Behavior{cfg: cfg}
}

type Decision struct {
	Sequence uint64
	InFlight int64
	Latency  time.Duration
	Fail     bool
}

func (b *Behavior) Admit() Decision {
	seq := b.seq.Add(1)
	inFlight := b.inFlight.Add(1)
	return Decision{
		Sequence: seq,
		InFlight: inFlight,
		Latency:  b.latency(seq, inFlight),
		Fail:     b.failure(seq),
	}
}

func (b *Behavior) Complete() {
	b.inFlight.Add(-1)
}

func (b *Behavior) InFlight() int64 {
	return b.inFlight.Load()
}

func (b *Behavior) latency(seq uint64, inFlight int64) time.Duration {
	total := b.cfg.Latency
	if b.cfg.Jitter > 0 {
		total += time.Duration(fraction(b.cfg.Seed, seq, streamJitter) * float64(b.cfg.Jitter))
	}
	return total + b.degradation(inFlight)
}

func (b *Behavior) degradation(inFlight int64) time.Duration {
	if b.cfg.DegradeAfter <= 0 || b.cfg.DegradeStep <= 0 || inFlight <= b.cfg.DegradeAfter {
		return 0
	}
	extra := time.Duration(inFlight-b.cfg.DegradeAfter) * b.cfg.DegradeStep
	if b.cfg.DegradeCap > 0 && extra > b.cfg.DegradeCap {
		return b.cfg.DegradeCap
	}
	return extra
}

func (b *Behavior) failure(seq uint64) bool {
	switch {
	case b.cfg.FailureRate <= 0:
		return false
	case b.cfg.FailureRate >= 1:
		return true
	}
	return fraction(b.cfg.Seed, seq, streamFailure) < b.cfg.FailureRate
}

type Quote struct {
	Partner         string `json:"partner"`
	QuoteID         string `json:"quote_id"`
	PremiumCents    int64  `json:"premium_cents"`
	Currency        string `json:"currency"`
	CoverageCents   int64  `json:"coverage_cents"`
	ValidForSeconds int64  `json:"valid_for_seconds"`
}

func (b *Behavior) Quote(request []byte) Quote {
	h := fnv.New64a()
	_, _ = h.Write([]byte(b.cfg.Name))
	_, _ = h.Write(request)
	sum := h.Sum64()

	return Quote{
		Partner:         b.cfg.Name,
		QuoteID:         fmt.Sprintf("%s-%016x", b.cfg.Name, sum),
		PremiumCents:    50000 + int64(sum%150001),
		Currency:        "BRL",
		CoverageCents:   3000000 + int64((sum>>32)%8)*1000000,
		ValidForSeconds: int64(b.cfg.QuoteTTL.Seconds()),
	}
}

const (
	streamFailure uint64 = 1
	streamJitter  uint64 = 2
)

func fraction(seed, seq, stream uint64) float64 {
	v := mix(mix(seed+seq*0x9e3779b97f4a7c15) ^ stream)
	return float64(v>>11) / float64(uint64(1)<<53)
}

func mix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
