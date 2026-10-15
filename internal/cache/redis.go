package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/partner"
	"github.com/GuilhermeOliveira591/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry"
)

// Cache define o contrato de persistência de cotações em cache.
type Cache interface {
	Get(ctx context.Context, tenant, key string) ([]partner.Quote, bool, error)
	Set(ctx context.Context, key string, quotes []partner.Quote, ttl time.Duration) error
}

// RedisCache implementa o Cache utilizando Redis.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCache inicializa o cliente Redis para cache de cotações.
func NewRedisCache(client *redis.Client, defaultTTL time.Duration) *RedisCache {
	if defaultTTL <= 0 {
		defaultTTL = 300 * time.Second
	}
	return &RedisCache{
		client: client,
		ttl:    defaultTTL,
	}
}

// BuildKey gera a chave canônica literal de cache garantindo o isolamento multi-tenant:
//
//	quote:v1:{tenant}:{coverage}:{plate}:{sha256(document)}
//
// Componentes obrigatórios:
// 1. tenant: identificador da corretora (X-Tenant-Id), impedindo contaminação cruzada;
// 2. coverage: modalidade da cobertura (comprehensive ou third_party);
// 3. plate: identificador do veículo em caixa alta normalizada;
// 4. sha256(document): hash SHA-256 do CPF/documento para pseudonimização nos termos da LGPD.
func BuildKey(tenant string, coverage string, plate string, document string) string {
	cleanTenant := strings.ToLower(strings.TrimSpace(tenant))
	cleanCoverage := strings.ToLower(strings.TrimSpace(coverage))
	cleanPlate := strings.ToUpper(strings.TrimSpace(plate))
	cleanDoc := strings.TrimSpace(document)

	h := sha256.Sum256([]byte(cleanDoc))
	docHash := hex.EncodeToString(h[:])

	return fmt.Sprintf("quote:v1:%s:%s:%s:%s", cleanTenant, cleanCoverage, cleanPlate, docHash)
}

// Get recupera as cotações do Redis, registrando métrica de hit ou miss com tenant.
func (c *RedisCache) Get(ctx context.Context, tenant, key string) ([]partner.Quote, bool, error) {
	if c == nil || c.client == nil {
		telemetry.GetMetrics().RecordCache(ctx, "miss", tenant)
		return nil, false, nil
	}

	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			telemetry.GetMetrics().RecordCache(ctx, "miss", tenant)
			return nil, false, nil
		}
		// Em caso de falha de conexão do Redis, fail-open (trata como miss e registra)
		telemetry.GetMetrics().RecordCache(ctx, "miss", tenant)
		return nil, false, err
	}

	var quotes []partner.Quote
	if err := json.Unmarshal([]byte(val), &quotes); err != nil {
		telemetry.GetMetrics().RecordCache(ctx, "miss", tenant)
		return nil, false, err
	}

	telemetry.GetMetrics().RecordCache(ctx, "hit", tenant)
	return quotes, true, nil
}

// Set armazena as cotações com o TTL estipulado.
func (c *RedisCache) Set(ctx context.Context, key string, quotes []partner.Quote, ttl time.Duration) error {
	if c == nil || c.client == nil || len(quotes) == 0 {
		return nil
	}
	if ttl <= 0 {
		ttl = c.ttl
	}

	data, err := json.Marshal(quotes)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}
