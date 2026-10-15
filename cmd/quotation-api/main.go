package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/cache"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/platform"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/quotation"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry"
)

func main() {
	cfg, err := platform.LoadConfig(platform.Environment())
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	ctx, stopListening := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopListening()

	stopTelemetry, err := platform.StartTelemetry(context.Background(), cfg.Telemetry)
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}

	// Inicializa métricas de negócio de Circuit Breaker, Cache e Latência
	_ = telemetry.GetMetrics()

	// Conexão com o cache Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	redisCache := cache.NewRedisCache(redisClient, cfg.CacheTTL)

	// Cliente de parceiras com Timeout e Circuit Breaker por parceira
	partnerClient := partner.NewClient(
		partner.WithTimeout(cfg.PartnerTimeout),
	)
	for _, p := range cfg.Partners {
		_ = partnerClient.BreakerState(p.Name)
	}

	// Serviço de cotações com Cache, Fallback (resposta parcial) e Paralelização
	service := quotation.NewService(
		cfg.Partners,
		partnerClient,
		quotation.WithCache(redisCache, cfg.CacheTTL),
		quotation.WithFallback(true),
		quotation.WithParallel(true),
	)

	api := quotation.NewAPI(service, cfg.Tenants)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           platform.InstrumentHandler(api.Routes(), cfg.Telemetry.ServiceName),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Print("quotation-api: signal received, shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("quotation-api: forced shutdown: %v", err)
		}
		if err := redisClient.Close(); err != nil {
			log.Printf("quotation-api: redis shutdown: %v", err)
		}
		if err := stopTelemetry(shutdown); err != nil {
			log.Printf("quotation-api: telemetry shutdown: %v", err)
		}
	}()

	log.Printf("quotation-api listening on %s | partners=%d brokers=%v redis=%s timeout=%v ttl=%v",
		server.Addr, len(cfg.Partners), cfg.Tenants, cfg.RedisAddr, cfg.PartnerTimeout, cfg.CacheTTL)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped: %v", err)
	}
}
