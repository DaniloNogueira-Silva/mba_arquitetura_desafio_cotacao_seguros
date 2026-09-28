package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(0)

	cfg, err := parseConfig(os.Args[1:], os.Getenv, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		log.Fatalf("loadgen: %v", err)
	}

	if err := run(cfg, os.Stdout); err != nil {
		log.Fatalf("loadgen: %v", err)
	}
}

func run(cfg Config, out io.Writer) error {
	bodies, err := quoteBodies(cfg.Distinct)
	if err != nil {
		return err
	}

	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopListening()
	ctx, expire := context.WithTimeout(ctx, cfg.Duration)
	defer expire()

	caller := newCaller(cfg, bodies)
	report := Report{Config: cfg}

	if cfg.Baseline > 0 {
		fmt.Fprintf(out, "baseline: %d requests, one at a time...\n", cfg.Baseline)
		phase := runPhase(ctx, caller, "baseline", cfg.Baseline, 1)
		report.Baseline = &phase
	}

	fmt.Fprintf(out, "load: %d requests, %d at a time...\n", cfg.Requests, cfg.Concurrency)
	report.Load = runPhase(ctx, caller, "load", cfg.Requests, cfg.Concurrency)

	if reason, down := unreachable(report.Baseline, &report.Load); down {
		return fmt.Errorf("no answer from %s (%s) — is the environment up? (make up)", cfg.Endpoint(), reason)
	}

	report.Write(out)
	return nil
}
