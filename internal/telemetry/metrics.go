package telemetry

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics encapsula os instrumentos OpenTelemetry de negócio da aplicação.
type Metrics struct {
	cacheRequests      metric.Int64Counter
	partnerDuration    metric.Float64Histogram
	breakerTransitions metric.Int64Counter

	mu            sync.RWMutex
	breakerStates map[string]int64 // 0: closed, 1: half-open, 2: open
}

var (
	globalMetrics *Metrics
	once          sync.Once
)

// GetMetrics retorna o singleton de métricas de negócio.
func GetMetrics() *Metrics {
	once.Do(func() {
		globalMetrics = newMetrics()
	})
	return globalMetrics
}

func newMetrics() *Metrics {
	meter := otel.GetMeterProvider().Meter("quotation-api")
	m := &Metrics{
		breakerStates: make(map[string]int64),
	}

	cacheRequests, _ := meter.Int64Counter(
		"quotation_cache_requests_total",
		metric.WithDescription("Total de requisições de cache particionadas por resultado (hit ou miss)"),
	)
	m.cacheRequests = cacheRequests

	partnerDuration, _ := meter.Float64Histogram(
		"quotation_partner_request_duration_seconds",
		metric.WithDescription("Latência das consultas às seguradoras parceiras com significado de domínio"),
		metric.WithUnit("s"),
	)
	m.partnerDuration = partnerDuration

	breakerTransitions, _ := meter.Int64Counter(
		"quotation_circuit_breaker_transitions_total",
		metric.WithDescription("Total de transições de estado do circuit breaker"),
	)
	m.breakerTransitions = breakerTransitions

	_, _ = meter.Int64ObservableGauge(
		"quotation_circuit_breaker_state",
		metric.WithDescription("Estado atual do circuit breaker (0=closed, 1=half-open, 2=open)"),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			m.mu.RLock()
			defer m.mu.RUnlock()
			for partner, state := range m.breakerStates {
				observer.Observe(state, metric.WithAttributes(attribute.String("partner", partner)))
			}
			return nil
		}),
	)

	return m
}

// RecordCache registra acerto ou erro no cache.
func (m *Metrics) RecordCache(ctx context.Context, result string, tenant string) {
	if m == nil || m.cacheRequests == nil {
		return
	}
	m.cacheRequests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("result", result),
		attribute.String("tenant_id", tenant),
	))
}

// RecordPartnerDuration registra a duração da consulta à parceira.
func (m *Metrics) RecordPartnerDuration(ctx context.Context, partner string, durationSeconds float64, status string) {
	if m == nil || m.partnerDuration == nil {
		return
	}
	m.partnerDuration.Record(ctx, durationSeconds, metric.WithAttributes(
		attribute.String("partner", partner),
		attribute.String("status", status),
	))
}

// SetBreakerState atualiza o estado observável do breaker e registra transição.
func (m *Metrics) SetBreakerState(partner string, state int64, fromState, toState string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.breakerStates[partner] = state
	m.mu.Unlock()

	if m.breakerTransitions != nil && fromState != "" && toState != "" {
		m.breakerTransitions.Add(context.Background(), 1, metric.WithAttributes(
			attribute.String("partner", partner),
			attribute.String("from", fromState),
			attribute.String("to", toState),
		))
	}
}
