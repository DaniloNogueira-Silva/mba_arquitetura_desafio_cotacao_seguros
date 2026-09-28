package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	Name          string
	Port          string
	Seed          uint64
	Latency       time.Duration
	Jitter        time.Duration
	FailureRate   float64
	FailureStatus int
	DegradeAfter  int64
	DegradeStep   time.Duration
	DegradeCap    time.Duration
	QuoteTTL      time.Duration
}

type environment func(string) string

func loadConfig(env environment) (Config, error) {
	cfg := Config{
		Name: env.text("PARTNER_NAME", "partner"),
		Port: env.text("PORT", "8080"),
	}

	var err error
	if cfg.Seed, err = env.unsignedInteger("PARTNER_SEED", 20260729); err != nil {
		return Config{}, err
	}
	if cfg.Latency, err = env.durationMs("PARTNER_LATENCY_MS", 0); err != nil {
		return Config{}, err
	}
	if cfg.Jitter, err = env.durationMs("PARTNER_JITTER_MS", 0); err != nil {
		return Config{}, err
	}
	if cfg.FailureRate, err = env.decimal("PARTNER_FAILURE_RATE", 0); err != nil {
		return Config{}, err
	}
	if cfg.DegradeStep, err = env.durationMs("PARTNER_DEGRADE_STEP_MS", 0); err != nil {
		return Config{}, err
	}
	if cfg.DegradeCap, err = env.durationMs("PARTNER_DEGRADE_CAP_MS", 0); err != nil {
		return Config{}, err
	}

	status, err := env.integer("PARTNER_FAILURE_STATUS", 503)
	if err != nil {
		return Config{}, err
	}
	cfg.FailureStatus = int(status)

	if cfg.DegradeAfter, err = env.integer("PARTNER_DEGRADE_AFTER", 0); err != nil {
		return Config{}, err
	}

	ttl, err := env.integer("PARTNER_QUOTE_TTL_SECONDS", 300)
	if err != nil {
		return Config{}, err
	}
	cfg.QuoteTTL = time.Duration(ttl) * time.Second

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch {
	case c.Name == "":
		return fmt.Errorf("PARTNER_NAME cannot be empty")
	case c.Port == "":
		return fmt.Errorf("PORT cannot be empty")
	case c.FailureRate < 0 || c.FailureRate > 1:
		return fmt.Errorf("PARTNER_FAILURE_RATE must be between 0 and 1, got %v", c.FailureRate)
	case c.FailureStatus < 400 || c.FailureStatus > 599:
		return fmt.Errorf("PARTNER_FAILURE_STATUS must be an error status (400-599), got %d", c.FailureStatus)
	case c.DegradeAfter < 0:
		return fmt.Errorf("PARTNER_DEGRADE_AFTER cannot be negative, got %d", c.DegradeAfter)
	case c.QuoteTTL <= 0:
		return fmt.Errorf("PARTNER_QUOTE_TTL_SECONDS must be greater than zero")
	case c.DegradeAfter > 0 && c.DegradeStep <= 0:
		return fmt.Errorf("PARTNER_DEGRADE_AFTER requires PARTNER_DEGRADE_STEP_MS greater than zero")
	}
	return nil
}

func (c Config) Summary() string {
	summary := fmt.Sprintf("latency=%s jitter=%s failure=%.0f%%", c.Latency, c.Jitter, c.FailureRate*100)
	if c.DegradeAfter > 0 {
		summary += fmt.Sprintf(" degrades=after %d in flight, +%s each (cap %s)", c.DegradeAfter, c.DegradeStep, c.DegradeCap)
	}
	return summary + fmt.Sprintf(" seed=%d", c.Seed)
}

func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name          string  `json:"partner"`
		Seed          uint64  `json:"seed"`
		LatencyMs     int64   `json:"latency_ms"`
		JitterMs      int64   `json:"jitter_ms"`
		FailureRate   float64 `json:"failure_rate"`
		FailureStatus int     `json:"failure_status"`
		DegradeAfter  int64   `json:"degrade_after"`
		DegradeStepMs int64   `json:"degrade_step_ms"`
		DegradeCapMs  int64   `json:"degrade_cap_ms"`
		TTLSeconds    int64   `json:"quote_ttl_seconds"`
	}{
		Name:          c.Name,
		Seed:          c.Seed,
		LatencyMs:     c.Latency.Milliseconds(),
		JitterMs:      c.Jitter.Milliseconds(),
		FailureRate:   c.FailureRate,
		FailureStatus: c.FailureStatus,
		DegradeAfter:  c.DegradeAfter,
		DegradeStepMs: c.DegradeStep.Milliseconds(),
		DegradeCapMs:  c.DegradeCap.Milliseconds(),
		TTLSeconds:    int64(c.QuoteTTL.Seconds()),
	})
}

func (e environment) text(key, fallback string) string {
	if v := e(key); v != "" {
		return v
	}
	return fallback
}

func (e environment) integer(key string, fallback int64) (int64, error) {
	v := e(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", key, v)
	}
	return n, nil
}

func (e environment) unsignedInteger(key string, fallback uint64) (uint64, error) {
	v := e(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", key, v)
	}
	return n, nil
}

func (e environment) decimal(key string, fallback float64) (float64, error) {
	v := e(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", key, v)
	}
	return n, nil
}

func (e environment) durationMs(key string, fallbackMs int64) (time.Duration, error) {
	ms, err := e.integer(key, fallbackMs)
	if err != nil {
		return 0, err
	}
	if ms < 0 {
		return 0, fmt.Errorf("%s cannot be negative, got %d", key, ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}
