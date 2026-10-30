package quotation

import (
	"context"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/cache"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

type memoryCache struct {
	store map[string][]partner.Quote
	gets  int
	sets  int
}

func newMemoryCache() *memoryCache {
	return &memoryCache{store: make(map[string][]partner.Quote)}
}

func (m *memoryCache) Get(_ context.Context, _, key string) ([]partner.Quote, bool, error) {
	m.gets++
	quotes, ok := m.store[key]
	return quotes, ok, nil
}

func (m *memoryCache) Set(_ context.Context, key string, quotes []partner.Quote, _ time.Duration) error {
	m.sets++
	m.store[key] = quotes
	return nil
}

func TestServiceServesFromCacheAndSkipsPartnerCalls(t *testing.T) {
	quoter := &fakeQuoter{premiums: defaultPremiums()}
	memCache := newMemoryCache()

	service := NewService(
		threePartners,
		quoter,
		WithCache(memCache, 5*time.Minute),
		WithFallback(true),
	)

	req := validRequest()

	// 1ª chamada: Cache Miss -> consulta as parceiras e popula o cache
	res1, err := service.Quote(context.Background(), "corretora-a", req)
	if err != nil {
		t.Fatalf("1ª Quote falhou: %v", err)
	}
	if res1.CacheHit {
		t.Errorf("1ª chamada deveria ser cache miss, mas retornou CacheHit=true")
	}
	if len(quoter.calls) != 3 {
		t.Fatalf("esperava 3 chamadas às parceiras na 1ª requisição, obteve %d", len(quoter.calls))
	}
	if memCache.sets != 1 {
		t.Fatalf("esperava 1 escrita no cache, obteve %d", memCache.sets)
	}

	// 2ª chamada: Cache Hit -> atende direto da memória sem chamar parceiras
	callsBefore := len(quoter.calls)
	res2, err := service.Quote(context.Background(), "corretora-a", req)
	if err != nil {
		t.Fatalf("2ª Quote falhou: %v", err)
	}
	if !res2.CacheHit {
		t.Errorf("2ª chamada deveria ser cache hit, mas retornou CacheHit=false")
	}
	if len(res2.Quotes) != 3 {
		t.Fatalf("esperava 3 cotações servidas do cache, obteve %d", len(res2.Quotes))
	}

	// Prova crucial: parceiras NÃO foram consultadas
	if len(quoter.calls) != callsBefore {
		t.Fatalf("cache hit chamou parceiras (%d novas chamadas)", len(quoter.calls)-callsBefore)
	}
}

func TestMultiTenantCacheIsolation(t *testing.T) {
	memCache := newMemoryCache()
	quoter := &fakeQuoter{premiums: defaultPremiums()}

	service := NewService(
		threePartners,
		quoter,
		WithCache(memCache, 5*time.Minute),
		WithFallback(true),
	)

	req := validRequest()

	// Corretora-A realiza cotação
	resA, err := service.Quote(context.Background(), "corretora-a", req)
	if err != nil {
		t.Fatalf("Quote corretora-a: %v", err)
	}
	if resA.TenantID != "corretora-a" {
		t.Errorf("tenant esperado corretora-a, obteve %s", resA.TenantID)
	}

	// Corretora-B cota exatamente o mesmo risco (mesmo veículo e motorista)
	callsBeforeB := len(quoter.calls)
	resB, err := service.Quote(context.Background(), "corretora-b", req)
	if err != nil {
		t.Fatalf("Quote corretora-b: %v", err)
	}

	// Corretora-B NÃO PODE ter tido Cache Hit da Corretora-A!
	if resB.CacheHit {
		t.Fatal("VIOLAÇÃO DE ISOLAMENTO: corretora-b recebeu resposta de cache da corretora-a!")
	}
	if len(quoter.calls) == callsBeforeB {
		t.Fatal("VIOLAÇÃO DE ISOLAMENTO: parceiras não foram chamadas para a corretora-b!")
	}

	// Confirma que as chaves no cache são distintas e contêm o tenant_id
	keyA := cache.BuildKey("corretora-a", req.Coverage, req.Vehicle.Plate, req.Driver.Document)
	keyB := cache.BuildKey("corretora-b", req.Coverage, req.Vehicle.Plate, req.Driver.Document)

	if keyA == keyB {
		t.Fatalf("chaves de cache idênticas para corretores diferentes: %s", keyA)
	}
	if _, ok := memCache.store[keyA]; !ok {
		t.Errorf("chave de A %s não encontrada no store", keyA)
	}
	if _, ok := memCache.store[keyB]; !ok {
		t.Errorf("chave de B %s não encontrada no store", keyB)
	}
}

func TestPartialFallbackWhenOnePartnerFails(t *testing.T) {
	// partner-flaky falha com 503
	quoter := &fakeQuoter{
		premiums: map[string]int64{
			"partner-slow":      180000,
			"partner-degrading": 120000,
		},
		failOn: "partner-flaky",
	}

	service := NewService(
		threePartners,
		quoter,
		WithFallback(true),
	)

	response, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("com fallback ativo, falha em 1 parceira não deveria retornar erro: %v", err)
	}

	// Contrato enriquecido:
	// Deve retornar 2 cotações válidas
	if len(response.Quotes) != 2 {
		t.Fatalf("esperava 2 cotações parciais, obteve %d", len(response.Quotes))
	}
	// Deve sinalizar degradação
	if !response.Degraded {
		t.Errorf("esperava Degraded=true na resposta parcial")
	}
	// Deve identificar a parceira indisponível
	if len(response.UnavailablePartners) != 1 || response.UnavailablePartners[0] != "partner-flaky" {
		t.Errorf("unavailable_partners esperado [partner-flaky], obteve %v", response.UnavailablePartners)
	}
	// As cotações sobreviventes devem estar ordenadas por preço crescente
	if response.Quotes[0].Partner != "partner-degrading" || response.Quotes[1].Partner != "partner-slow" {
		t.Errorf("ordenação incorreta de cotações parciais: %+v", response.Quotes)
	}
}

func TestFallbackWhenAllPartnersFailReturns502(t *testing.T) {
	// Todas as parceiras falham
	allFailingPartners := []platform.Partner{
		{Name: "p1", BaseURL: "http://p1"},
		{Name: "p2", BaseURL: "http://p2"},
	}
	quoter := &fakeQuoter{failOn: "p1"} // e faremos ambas falharem

	service := NewService(allFailingPartners[:1], quoter, WithFallback(true))
	_, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err == nil {
		t.Fatal("esperava erro 502 quando todas as parceiras falham, mas obteve sucesso")
	}
}

func TestParallelAggregationReducesTotalLatency(t *testing.T) {
	delay := 60 * time.Millisecond
	quoter := &fakeQuoter{delay: delay, premiums: defaultPremiums()}

	service := NewService(
		threePartners,
		quoter,
		WithParallel(true),
		WithFallback(true),
	)

	start := time.Now()
	res, err := service.Quote(context.Background(), "corretora-a", validRequest())
	if err != nil {
		t.Fatalf("Quote paralela falhou: %v", err)
	}
	elapsed := time.Since(start)

	if len(res.Quotes) != 3 {
		t.Fatalf("esperava 3 cotações, obteve %d", len(res.Quotes))
	}

	// 3 parceiras com 60ms cada em série levariam >= 180ms.
	// Em paralelo, a latência esperada é max(60ms, 60ms, 60ms) ~ 60-100ms.
	maxExpected := 140 * time.Millisecond
	if elapsed > maxExpected {
		t.Errorf("agregação paralela levou %v, esperado menor que %v", elapsed, maxExpected)
	}
	if peak := quoter.peak.Load(); peak < 2 {
		t.Errorf("pico de concorrência foi %d, esperado >= 2 para execução paralela", peak)
	}
}

func TestSpanAttributesDoNotLeakPlateOrDocumentOrCacheKey(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	tracer := tp.Tracer("test")

	quoter := &fakeQuoter{premiums: defaultPremiums()}
	memCache := newMemoryCache()
	service := NewService(
		threePartners,
		quoter,
		WithCache(memCache, 5*time.Minute),
		WithFallback(true),
	)

	req := validRequest() // Plate: "ABC1D23", Document: "12345678900"

	// 1ª chamada (cache miss) e 2ª chamada (cache hit)
	for i, expectedHit := range []bool{false, true} {
		ctx, span := tracer.Start(context.Background(), "POST /quotes")
		_, err := service.Quote(ctx, "corretora-a", req)
		span.End()
		if err != nil {
			t.Fatalf("chamada %d falhou: %v", i+1, err)
		}

		spans := exporter.GetSpans()
		if len(spans) != i+1 {
			t.Fatalf("esperava %d spans gravados, obteve %d", i+1, len(spans))
		}
		recorded := spans[i]

		var foundCacheHit, foundTenant bool
		for _, attr := range recorded.Attributes {
			key := string(attr.Key)
			val := attr.Value.Emit()

			if key == "quotation.cache_key" {
				t.Errorf("span não deve conter o atributo quotation.cache_key, mas encontrou %q", val)
			}
			if strings.Contains(val, req.Vehicle.Plate) || strings.Contains(val, req.Driver.Document) {
				t.Errorf("atributo %s=%q vaza placa ou documento no span", key, val)
			}
			if key == "quotation.cache_hit" {
				foundCacheHit = true
				if attr.Value.AsBool() != expectedHit {
					t.Errorf("chamada %d: quotation.cache_hit=%v, esperado %v", i+1, attr.Value.AsBool(), expectedHit)
				}
			}
			if key == "tenant" {
				foundTenant = true
				if attr.Value.AsString() != "corretora-a" {
					t.Errorf("chamada %d: tenant=%q, esperado corretora-a", i+1, attr.Value.AsString())
				}
			}
		}

		if !foundCacheHit || !foundTenant {
			t.Errorf("chamada %d: esperado quotation.cache_hit e tenant nos atributos do span (foundCacheHit=%v, foundTenant=%v)", i+1, foundCacheHit, foundTenant)
		}
	}
}

