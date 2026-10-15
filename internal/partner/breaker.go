package partner

import (
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry"
)

// BreakerConfig define os limiares de resiliência de um circuit breaker.
type BreakerConfig struct {
	ConsecutiveFailures uint32
	OpenTimeout         time.Duration
	HalfOpenMaxRequests uint32
	Interval            time.Duration
}

// DefaultBreakerConfig retorna a configuração defendida no SAD:
// - Abre após 5 falhas consecutivas (cobre a rajada determinística 49-57 da partner-flaky)
// - Permanece aberto por 5 segundos antes de passar a half-open
// - Permite 1 requisição de teste em half-open
// - Janela cíclica de 10 segundos para resetar contadores em estado fechado
func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		ConsecutiveFailures: 5,
		OpenTimeout:         5 * time.Second,
		HalfOpenMaxRequests: 1,
		Interval:            10 * time.Second,
	}
}

// newPartnerBreaker cria um circuit breaker dedicado para uma seguradora parceira.
func newPartnerBreaker(partnerName string, cfg BreakerConfig) *gobreaker.CircuitBreaker[Quote] {
	telemetry.GetMetrics().SetBreakerState(partnerName, 0, "", "")
	return gobreaker.NewCircuitBreaker[Quote](gobreaker.Settings{
		Name:        partnerName,
		MaxRequests: cfg.HalfOpenMaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.OpenTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailures
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			var stateVal int64
			switch to {
			case gobreaker.StateClosed:
				stateVal = 0
			case gobreaker.StateHalfOpen:
				stateVal = 1
			case gobreaker.StateOpen:
				stateVal = 2
			}
			telemetry.GetMetrics().SetBreakerState(name, stateVal, from.String(), to.String())
		},
	})
}
