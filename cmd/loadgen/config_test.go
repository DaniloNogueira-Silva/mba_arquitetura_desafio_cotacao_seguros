package main

import (
	"io"
	"testing"
	"time"
)

func env(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestDefaultsReproduceTheScenarioWithNoArguments(t *testing.T) {
	cfg, err := parseConfig(nil, env(nil), io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	if cfg.Endpoint() != "http://localhost:8080/quotes" {
		t.Errorf("endpoint %q, expected the local quotation-api", cfg.Endpoint())
	}
	if cfg.Tenant != defaultTenant {
		t.Errorf("tenant %q, expected %q", cfg.Tenant, defaultTenant)
	}

	if cfg.Concurrency < 40 {
		t.Errorf("default concurrency %d is below the calibrated load", cfg.Concurrency)
	}
	if cfg.Requests < cfg.Concurrency*2 {
		t.Errorf("%d requests at %d in flight does not reach the degradation plateau",
			cfg.Requests, cfg.Concurrency)
	}
	if cfg.Baseline < 1 {
		t.Error("with no baseline there is nothing to compare the load against")
	}
}

func TestEnvironmentPointsTheRunAndFlagsWin(t *testing.T) {
	vars := env(map[string]string{
		"LOADGEN_TARGET": "http://quotation-api:8080/",
		"LOADGEN_TENANT": "corretora-b",
	})

	cfg, err := parseConfig(nil, vars, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	if cfg.Endpoint() != "http://quotation-api:8080/quotes" {
		t.Errorf("endpoint %q, expected the address from the environment with no double slash", cfg.Endpoint())
	}
	if cfg.Tenant != "corretora-b" {
		t.Errorf("tenant %q, expected the one from the environment", cfg.Tenant)
	}

	cfg, err = parseConfig([]string{"-target", "http://127.0.0.1:9999", "-tenant", "corretora-a"}, vars, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Endpoint() != "http://127.0.0.1:9999/quotes" || cfg.Tenant != "corretora-a" {
		t.Errorf("flag did not override the environment: %s / %s", cfg.Endpoint(), cfg.Tenant)
	}
}

func TestFlagsShapeTheRun(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-concurrency", "8", "-requests", "16", "-baseline", "0",
		"-duration", "30s", "-timeout", "0", "-distinct", "1",
	}, env(nil), io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}

	want := Config{
		Target: defaultTarget, Tenant: defaultTenant,
		Concurrency: 8, Requests: 16, Baseline: 0,
		Duration: 30 * time.Second, Timeout: 0, Distinct: 1,
	}
	if cfg != want {
		t.Fatalf("config read %+v, expected %+v", cfg, want)
	}
}

func TestInvalidRunIsRefusedInsteadOfMeasuringNothing(t *testing.T) {
	cases := map[string][]string{
		"target without scheme":  {"-target", "localhost:8080"},
		"unparseable target":     {"-target", "http://%zz"},
		"empty tenant":           {"-tenant", "  "},
		"no concurrency":         {"-concurrency", "0"},
		"negative concurrency":   {"-concurrency", "-1"},
		"no requests":            {"-requests", "0"},
		"negative baseline":      {"-baseline", "-1"},
		"zeroed duration":        {"-duration", "0"},
		"negative timeout":       {"-timeout", "-1s"},
		"no distinct quote":      {"-distinct", "0"},
		"unknown flag":           {"-rps", "10"},
		"duration without units": {"-duration", "30"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(args, env(nil), io.Discard); err == nil {
				t.Fatalf("invalid run (%v) was accepted", args)
			}
		})
	}
}
