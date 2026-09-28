package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	Target      string
	Tenant      string
	Concurrency int
	Requests    int
	Baseline    int
	Duration    time.Duration
	Timeout     time.Duration
	Distinct    int
}

const (
	defaultTarget = "http://localhost:8080"
	defaultTenant = "corretora-a"
)

const (
	defaultConcurrency = 50
	defaultRequests    = 200
	defaultBaseline    = 10
	defaultDuration    = 2 * time.Minute
	defaultTimeout     = 30 * time.Second
	defaultDistinct    = 5
)

func parseConfig(args []string, env func(string) string, out io.Writer) (Config, error) {
	flags := flag.NewFlagSet("loadgen", flag.ContinueOnError)
	flags.SetOutput(out)

	var cfg Config
	flags.StringVar(&cfg.Target, "target", text(env, "LOADGEN_TARGET", defaultTarget),
		"base URL of the quotation-api")
	flags.StringVar(&cfg.Tenant, "tenant", text(env, "LOADGEN_TENANT", defaultTenant),
		"broker sent in the X-Tenant-Id header")
	flags.IntVar(&cfg.Concurrency, "concurrency", defaultConcurrency,
		"requests in flight during the load phase")
	flags.IntVar(&cfg.Requests, "requests", defaultRequests,
		"total requests in the load phase")
	flags.IntVar(&cfg.Baseline, "baseline", defaultBaseline,
		"sequential requests measured before the load (0 skips the baseline)")
	flags.DurationVar(&cfg.Duration, "duration", defaultDuration,
		"time limit for the run; whatever is missing is not sent")
	flags.DurationVar(&cfg.Timeout, "timeout", defaultTimeout,
		"limit per request (0 waits forever, like the starter's API does with its partners)")
	flags.IntVar(&cfg.Distinct, "distinct", defaultDistinct,
		"how many different quotes to rotate through; with -distinct 1 every request is the same one")

	flags.Usage = func() {
		fmt.Fprintf(out, "loadgen reproduces the failure scenario of the starter against the quotation-api.\n\n")
		fmt.Fprintf(out, "Usage: loadgen [flags]\n\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.Target = strings.TrimRight(strings.TrimSpace(cfg.Target), "/")
	cfg.Tenant = strings.TrimSpace(cfg.Tenant)
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	if u, err := url.Parse(c.Target); err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("-target: %q is not an absolute URL", c.Target)
	}

	switch {
	case c.Tenant == "":
		return fmt.Errorf("-tenant cannot be empty")
	case c.Concurrency < 1:
		return fmt.Errorf("-concurrency must be at least 1, got %d", c.Concurrency)
	case c.Requests < 1:
		return fmt.Errorf("-requests must be at least 1, got %d", c.Requests)
	case c.Baseline < 0:
		return fmt.Errorf("-baseline cannot be negative, got %d", c.Baseline)
	case c.Duration <= 0:
		return fmt.Errorf("-duration must be greater than zero, got %s", c.Duration)
	case c.Timeout < 0:
		return fmt.Errorf("-timeout cannot be negative, got %s", c.Timeout)
	case c.Distinct < 1:
		return fmt.Errorf("-distinct must be at least 1, got %d", c.Distinct)
	}
	return nil
}

func (c Config) Endpoint() string { return c.Target + "/quotes" }

func text(env func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(env(key)); v != "" {
		return v
	}
	return fallback
}
