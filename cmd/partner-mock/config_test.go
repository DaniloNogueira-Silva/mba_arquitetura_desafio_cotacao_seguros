package main

import (
	"testing"
	"time"
)

func env(pairs map[string]string) environment {
	return func(key string) string { return pairs[key] }
}

func TestDefaultConfigIsAWellBehavedPartner(t *testing.T) {
	cfg, err := loadConfig(env(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if cfg.Name != "partner" || cfg.Port != "8080" {
		t.Errorf("name %q and port %q, expected partner and 8080", cfg.Name, cfg.Port)
	}
	if cfg.Latency != 0 || cfg.Jitter != 0 || cfg.FailureRate != 0 || cfg.DegradeAfter != 0 {
		t.Errorf("with no configuration the partner should be well behaved, got %+v", cfg)
	}
	if cfg.FailureStatus != 503 {
		t.Errorf("failure status %d, expected 503", cfg.FailureStatus)
	}
	if cfg.QuoteTTL != 300*time.Second {
		t.Errorf("quote TTL %s, expected 5m", cfg.QuoteTTL)
	}
}

func TestConfigReadsTheFullProfile(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{
		"PARTNER_NAME":              "partner-degrading",
		"PORT":                      "9003",
		"PARTNER_SEED":              "7",
		"PARTNER_LATENCY_MS":        "120",
		"PARTNER_JITTER_MS":         "40",
		"PARTNER_FAILURE_RATE":      "0.25",
		"PARTNER_FAILURE_STATUS":    "502",
		"PARTNER_DEGRADE_AFTER":     "5",
		"PARTNER_DEGRADE_STEP_MS":   "300",
		"PARTNER_DEGRADE_CAP_MS":    "6000",
		"PARTNER_QUOTE_TTL_SECONDS": "60",
	}))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	want := Config{
		Name:          "partner-degrading",
		Port:          "9003",
		Seed:          7,
		Latency:       120 * time.Millisecond,
		Jitter:        40 * time.Millisecond,
		FailureRate:   0.25,
		FailureStatus: 502,
		DegradeAfter:  5,
		DegradeStep:   300 * time.Millisecond,
		DegradeCap:    6 * time.Second,
		QuoteTTL:      60 * time.Second,
	}
	if cfg != want {
		t.Fatalf("config read %+v, expected %+v", cfg, want)
	}
}

func TestInvalidConfigFailsInsteadOfFallingBackToTheDefault(t *testing.T) {
	cases := map[string]map[string]string{
		"failure rate above 1":           {"PARTNER_FAILURE_RATE": "40"},
		"negative failure rate":          {"PARTNER_FAILURE_RATE": "-0.1"},
		"non-numeric failure rate":       {"PARTNER_FAILURE_RATE": "forty percent"},
		"negative latency":               {"PARTNER_LATENCY_MS": "-1"},
		"non-integer latency":            {"PARTNER_LATENCY_MS": "150ms"},
		"failure status out of range":    {"PARTNER_FAILURE_STATUS": "200"},
		"negative degradation threshold": {"PARTNER_DEGRADE_AFTER": "-3"},
		"degradation without step":       {"PARTNER_DEGRADE_AFTER": "5"},
		"zeroed quote TTL":               {"PARTNER_QUOTE_TTL_SECONDS": "0"},
		"non-numeric seed":               {"PARTNER_SEED": "abc"},
	}

	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadConfig(env(vars)); err == nil {
				t.Fatalf("invalid configuration (%v) was accepted", vars)
			}
		})
	}
}
