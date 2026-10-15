package partner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
)

func TestCircuitBreakerOpensAndShortCircuitsWithoutCallingPartner(t *testing.T) {
	var httpCalls atomic.Int32
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		if shouldFail.Load() {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"partner":"partner-flaky","quote_id":"q-test","premium_cents":100000,"currency":"BRL","coverage_cents":5000000,"valid_for_seconds":300}`))
	}))
	defer server.Close()

	partner := platform.Partner{Name: "partner-flaky", BaseURL: server.URL}

	// Configuração determinística de teste:
	// - Abre após 5 falhas consecutivas
	// - Timeout do circuito aberto: 50ms
	cfg := BreakerConfig{
		ConsecutiveFailures: 5,
		OpenTimeout:         50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
		Interval:            1 * time.Minute,
	}

	client := NewClient(
		WithTimeout(500*time.Millisecond),
		WithBreakerConfig(cfg),
	)

	// Estado inicial: Fechado
	if state := client.BreakerState("partner-flaky"); state != gobreaker.StateClosed {
		t.Fatalf("estado inicial esperado StateClosed, obteve %v", state)
	}

	// Dispara 5 falhas consecutivas
	for i := 1; i <= 5; i++ {
		_, err := client.Quote(context.Background(), partner, map[string]string{"broker": "corretora-a"})
		if err == nil {
			t.Fatalf("chamada %d esperava erro 503, obteve sucesso", i)
		}
	}

	if calls := httpCalls.Load(); calls != 5 {
		t.Fatalf("esperava 5 chamadas HTTP ao servidor, obteve %d", calls)
	}

	// Agora o circuito DEVE estar ABERTO
	if state := client.BreakerState("partner-flaky"); state != gobreaker.StateOpen {
		t.Fatalf("esperava StateOpen após 5 falhas, obteve %v", state)
	}

	// Próximas 5 chamadas com circuito ABERTO:
	// Devem retornar erro imediato sem chamar o HTTP server!
	callsBefore := httpCalls.Load()
	for i := 1; i <= 5; i++ {
		_, err := client.Quote(context.Background(), partner, map[string]string{"broker": "corretora-a"})
		if err == nil {
			t.Fatal("esperava erro de circuito aberto, obteve sucesso")
		}
	}

	// Prova crucial: o número de chamadas HTTP NÃO PODE ter aumentado!
	callsAfter := httpCalls.Load()
	if callsAfter != callsBefore {
		t.Fatalf("circuito aberto realizou %d chamadas HTTP extras; aberto significa NÃO chamar a parceira", callsAfter-callsBefore)
	}

	// Aguarda o open timeout (50ms) para transição para Half-Open
	time.Sleep(60 * time.Millisecond)

	// Prepara o servidor para responder sucesso
	shouldFail.Store(false)

	// Chamada de sonda em Half-Open
	quote, err := client.Quote(context.Background(), partner, map[string]string{"broker": "corretora-a"})
	if err != nil {
		t.Fatalf("chamada de sonda em Half-Open falhou: %v", err)
	}
	if quote.QuoteID != "q-test" {
		t.Errorf("quote_id inesperado: %s", quote.QuoteID)
	}

	// Agora o circuito DEVE ter retornado para FECHADO
	if state := client.BreakerState("partner-flaky"); state != gobreaker.StateClosed {
		t.Fatalf("após sucesso em Half-Open, esperava StateClosed, obteve %v", state)
	}
}

func TestTimeoutCountsAsFailureForCircuitBreaker(t *testing.T) {
	// Simula parceira degradante com latência alta (> timeout)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Mais lento que o timeout de 30ms
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"partner":"partner-degrading","quote_id":"q-deg","premium_cents":100000}`))
	}))
	defer server.Close()

	partner := platform.Partner{Name: "partner-degrading", BaseURL: server.URL}

	cfg := BreakerConfig{
		ConsecutiveFailures: 3,
		OpenTimeout:         100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
		Interval:            1 * time.Minute,
	}

	client := NewClient(
		WithTimeout(30*time.Millisecond), // Timeout menor que a latência
		WithBreakerConfig(cfg),
	)

	// 3 chamadas que estouram timeout
	for i := 1; i <= 3; i++ {
		_, err := client.Quote(context.Background(), partner, map[string]string{})
		if err == nil {
			t.Fatalf("chamada %d esperava erro de timeout", i)
		}
	}

	// Circuit breaker deve ter aberto em função dos timeouts!
	if state := client.BreakerState("partner-degrading"); state != gobreaker.StateOpen {
		t.Fatalf("estouro de timeout não abriu o circuito: estado atual %v", state)
	}
}
