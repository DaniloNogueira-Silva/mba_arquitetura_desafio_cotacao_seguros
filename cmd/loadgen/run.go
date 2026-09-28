package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/quotation"
)

type Result struct {
	Latency time.Duration
	Status  int
	Partner string
	Failure string
}

func (r Result) OK() bool { return r.Status == http.StatusOK }

type caller struct {
	http     *http.Client
	endpoint string
	tenant   string
	timeout  time.Duration
	bodies   [][]byte
}

func newCaller(cfg Config, bodies [][]byte) *caller {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = cfg.Concurrency
	transport.MaxIdleConnsPerHost = cfg.Concurrency

	return &caller{
		http:     &http.Client{Transport: transport},
		endpoint: cfg.Endpoint(),
		tenant:   cfg.Tenant,
		timeout:  cfg.Timeout,
		bodies:   bodies,
	}
}

func (c *caller) call(ctx context.Context, n int) (Result, bool) {
	requestCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	body := c.bodies[n%len(c.bodies)]
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{Failure: err.Error()}, true
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(quotation.TenantHeader, c.tenant)

	started := time.Now()
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, false
		}
		return Result{Latency: time.Since(started), Failure: reason(err)}, true
	}
	defer func() { _ = response.Body.Close() }()

	result := Result{Status: response.StatusCode}
	if !result.OK() {
		var failure platform.ErrorBody
		if err := json.NewDecoder(io.LimitReader(response.Body, responseLimit)).Decode(&failure); err == nil {
			result.Partner = failure.Partner
		}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, responseLimit))
	result.Latency = time.Since(started)
	return result, true
}

const responseLimit = 1 << 20

func reason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		var netErr *net.OpError
		if errors.As(urlErr.Err, &netErr) {
			return netErr.Err.Error()
		}
		return urlErr.Err.Error()
	}
	return err.Error()
}

type Phase struct {
	Name        string
	Planned     int
	Concurrency int
	Started     time.Time
	Ended       time.Time
	Results     []Result
}

func runPhase(ctx context.Context, c *caller, name string, requests, concurrency int) Phase {
	phase := Phase{Name: name, Planned: requests, Concurrency: concurrency, Started: time.Now()}

	jobs := make(chan int)
	results := make(chan Result, requests)

	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for n := range jobs {
				if result, counted := c.call(ctx, n); counted {
					results <- result
				}
			}
		}()
	}

dispatch:
	for n := 0; n < requests; n++ {
		select {
		case jobs <- n:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	close(results)

	phase.Ended = time.Now()
	phase.Results = make([]Result, 0, requests)
	for result := range results {
		phase.Results = append(phase.Results, result)
	}
	return phase
}
