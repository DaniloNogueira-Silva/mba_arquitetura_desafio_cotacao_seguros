package quotation

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/cache"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

type Quoter interface {
	Quote(ctx context.Context, p platform.Partner, request any) (partner.Quote, error)
}

type Service struct {
	partners        []platform.Partner
	quoter          Quoter
	cache           cache.Cache
	cacheTTL        time.Duration
	fallbackEnabled bool
	parallelEnabled bool
}

type ServiceOption func(*Service)

// WithCache habilita o cache de cotações com a persistência e TTL informados.
func WithCache(c cache.Cache, ttl time.Duration) ServiceOption {
	return func(s *Service) {
		s.cache = c
		s.cacheTTL = ttl
	}
}

// WithFallback habilita a resposta parcial degradada quando parceiras falharem.
func WithFallback(enabled bool) ServiceOption {
	return func(s *Service) {
		s.fallbackEnabled = enabled
	}
}

// WithParallel habilita a consulta paralela às parceiras sobrepondo as latências.
func WithParallel(enabled bool) ServiceOption {
	return func(s *Service) {
		s.parallelEnabled = enabled
	}
}

func NewService(partners []platform.Partner, quoter Quoter, opts ...ServiceOption) *Service {
	s := &Service{
		partners: partners,
		quoter:   quoter,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Quote(ctx context.Context, tenant string, request Request) (Response, error) {
	start := time.Now()
	span := trace.SpanFromContext(ctx)

	// 1. Verificação no Cache (se configurado)
	var cacheKey string
	if s.cache != nil {
		cacheKey = cache.BuildKey(tenant, request.Coverage, request.Vehicle.Plate, request.Driver.Document)
		if cachedQuotes, found, _ := s.cache.Get(ctx, tenant, cacheKey); found {
			if span.IsRecording() {
				span.SetAttributes(
					attribute.Bool("quotation.cache_hit", true),
					attribute.String("tenant", tenant),
				)
			}
			return Response{
				TenantID:  tenant,
				Quotes:    cachedQuotes,
				ElapsedMs: time.Since(start).Milliseconds(),
				CacheHit:  true,
			}, nil
		}
		if span.IsRecording() {
			span.SetAttributes(
				attribute.Bool("quotation.cache_hit", false),
				attribute.String("tenant", tenant),
			)
		}
	}

	forPartner := partnerRequest{Broker: tenant, Request: request}

	var quotes []partner.Quote
	var unavailable []string
	var lastErr error

	if s.parallelEnabled {
		quotes, unavailable, lastErr = s.quoteParallel(ctx, forPartner)
	} else {
		quotes, unavailable, lastErr = s.quoteSerial(ctx, forPartner)
	}

	// Sem fallback habilitado: qualquer erro aborta a requisição (comportamento baseline)
	if !s.fallbackEnabled && lastErr != nil {
		return Response{}, lastErr
	}

	// Com fallback habilitado: se todas falharam, retorna o último erro
	if len(quotes) == 0 && lastErr != nil {
		return Response{}, lastErr
	}

	// Ordenação obrigatória das propostas da mais barata para a mais cara
	sort.Slice(quotes, func(i, j int) bool {
		return quotes[i].PremiumCents < quotes[j].PremiumCents
	})

	// 3. Armazena no cache se cotações válidas foram obtidas
	if s.cache != nil && len(quotes) > 0 {
		_ = s.cache.Set(ctx, cacheKey, quotes, s.cacheTTL)
	}

	return Response{
		TenantID:            tenant,
		Quotes:              quotes,
		ElapsedMs:           time.Since(start).Milliseconds(),
		Degraded:            len(unavailable) > 0,
		UnavailablePartners: unavailable,
		CacheHit:            false,
	}, nil
}

func (s *Service) quoteSerial(ctx context.Context, forPartner partnerRequest) ([]partner.Quote, []string, error) {
	quotes := make([]partner.Quote, 0, len(s.partners))
	var unavailable []string
	var lastErr error

	for _, p := range s.partners {
		quote, err := s.quoter.Quote(ctx, p, forPartner)
		if err != nil {
			lastErr = err
			unavailable = append(unavailable, p.Name)
			if !s.fallbackEnabled {
				return nil, nil, err
			}
			continue
		}
		quotes = append(quotes, quote)
	}
	return quotes, unavailable, lastErr
}

func (s *Service) quoteParallel(ctx context.Context, forPartner partnerRequest) ([]partner.Quote, []string, error) {
	type partnerResult struct {
		partner platform.Partner
		quote   partner.Quote
		err     error
	}

	results := make(chan partnerResult, len(s.partners))
	var wg sync.WaitGroup

	for _, p := range s.partners {
		wg.Add(1)
		go func(partner platform.Partner) {
			defer wg.Done()
			q, err := s.quoter.Quote(ctx, partner, forPartner)
			results <- partnerResult{partner: partner, quote: q, err: err}
		}(p)
	}

	wg.Wait()
	close(results)

	quotes := make([]partner.Quote, 0, len(s.partners))
	var unavailable []string
	var lastErr error

	for res := range results {
		if res.err != nil {
			lastErr = res.err
			unavailable = append(unavailable, res.partner.Name)
		} else {
			quotes = append(quotes, res.quote)
		}
	}

	sort.Strings(unavailable)
	return quotes, unavailable, lastErr
}
