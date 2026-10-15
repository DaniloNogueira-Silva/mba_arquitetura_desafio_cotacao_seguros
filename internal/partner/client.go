package partner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry"
)

type Quote struct {
	Partner         string `json:"partner"`
	QuoteID         string `json:"quote_id"`
	PremiumCents    int64  `json:"premium_cents"`
	Currency        string `json:"currency"`
	CoverageCents   int64  `json:"coverage_cents"`
	ValidForSeconds int64  `json:"valid_for_seconds"`
}

type Client struct {
	http          *http.Client
	timeout       time.Duration
	breakerConfig BreakerConfig
	breakers      map[string]*gobreaker.CircuitBreaker[Quote]
	mu            sync.RWMutex
}

type ClientOption func(*Client)

func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

func WithBreakerConfig(cfg BreakerConfig) ClientOption {
	return func(c *Client) {
		c.breakerConfig = cfg
	}
}

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.http = httpClient
		}
	}
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		http:          &http.Client{Transport: platform.InstrumentTransport(http.DefaultTransport)},
		timeout:       2500 * time.Millisecond,
		breakerConfig: DefaultBreakerConfig(),
		breakers:      make(map[string]*gobreaker.CircuitBreaker[Quote]),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) getBreaker(partnerName string) *gobreaker.CircuitBreaker[Quote] {
	c.mu.RLock()
	cb, ok := c.breakers[partnerName]
	c.mu.RUnlock()
	if ok {
		return cb
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if cb, ok = c.breakers[partnerName]; ok {
		return cb
	}
	cb = newPartnerBreaker(partnerName, c.breakerConfig)
	c.breakers[partnerName] = cb
	return cb
}

// BreakerState retorna o estado atual do circuit breaker da parceira informada.
func (c *Client) BreakerState(partnerName string) gobreaker.State {
	return c.getBreaker(partnerName).State()
}

const responseLimit = 1 << 20

func (c *Client) Quote(ctx context.Context, p platform.Partner, request any) (Quote, error) {
	cb := c.getBreaker(p.Name)
	span := trace.SpanFromContext(ctx)

	quote, err := cb.Execute(func() (Quote, error) {
		start := time.Now()
		reqCtx := ctx
		var cancel context.CancelFunc
		if c.timeout > 0 {
			reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}

		q, reqErr := c.doRequest(reqCtx, p, request)
		duration := time.Since(start).Seconds()

		statusStr := "200"
		if reqErr != nil {
			var pe *Error
			if errors.As(reqErr, &pe) && pe.Status != 0 {
				statusStr = fmt.Sprintf("%d", pe.Status)
			} else if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
				statusStr = "timeout"
			} else {
				statusStr = "error"
			}
		}
		telemetry.GetMetrics().RecordPartnerDuration(ctx, p.Name, duration, statusStr)

		return q, reqErr
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			if span.IsRecording() {
				span.SetAttributes(
					attribute.String("partner.circuit_breaker.state", "open"),
					attribute.Bool("partner.short_circuited", true),
				)
			}
			return Quote{}, &Error{
				Partner: p.Name,
				Status:  http.StatusServiceUnavailable,
				Reason:  "circuit breaker is open (short-circuited)",
			}
		}
		return Quote{}, err
	}

	return quote, nil
}

func (c *Client) doRequest(ctx context.Context, p platform.Partner, request any) (Quote, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: fmt.Sprintf("invalid request: %v", err)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/quotes", bytes.NewReader(body))
	if err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Quote{}, &Error{
				Partner: p.Name,
				Status:  http.StatusGatewayTimeout,
				Reason:  fmt.Sprintf("timeout of %v exceeded", c.timeout),
			}
		}
		return Quote{}, &Error{Partner: p.Name, Reason: err.Error()}
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, responseLimit))
		return Quote{}, &Error{
			Partner: p.Name,
			Status:  response.StatusCode,
			Reason:  fmt.Sprintf("partner replied %d", response.StatusCode),
		}
	}

	var quote Quote
	if err := json.NewDecoder(io.LimitReader(response.Body, responseLimit)).Decode(&quote); err != nil {
		return Quote{}, &Error{Partner: p.Name, Reason: fmt.Sprintf("unreadable response: %v", err)}
	}

	quote.Partner = p.Name
	return quote, nil
}

type Error struct {
	Partner string
	Status  int
	Reason  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("partner %s: %s", e.Partner, e.Reason)
}
