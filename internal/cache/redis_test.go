package cache

import (
	"strings"
	"testing"
)

func TestBuildKeyContainsTenantAndNormalizesFields(t *testing.T) {
	tenant := " Corretora-Alfa "
	coverage := " Comprehensive "
	plate := " abc1d23 "
	doc := " 12345678901 "

	key := BuildKey(tenant, coverage, plate, doc)

	// Regra 1: Deve conter o tenant isolado
	if !strings.HasPrefix(key, "quote:v1:corretora-alfa:") {
		t.Fatalf("chave não inicia com prefixo e tenant normalizado: %s", key)
	}

	// Regra 2: Deve conter a cobertura
	if !strings.Contains(key, ":comprehensive:") {
		t.Fatalf("chave não contém cobertura normalizada: %s", key)
	}

	// Regra 3: Deve conter a placa em caixa alta
	if !strings.Contains(key, ":ABC1D23:") {
		t.Fatalf("chave não contém placa em caixa alta: %s", key)
	}

	// Regra 4: Não deve conter o CPF em texto claro (LGPD minimização - apenas hash SHA-256)
	if strings.Contains(key, "12345678901") {
		t.Fatalf("chave contém CPF em texto claro, violando LGPD: %s", key)
	}
}

func TestDifferentTenantsProduceDifferentKeys(t *testing.T) {
	keyA := BuildKey("corretora-a", "comprehensive", "ABC1D23", "12345678901")
	keyB := BuildKey("corretora-b", "comprehensive", "ABC1D23", "12345678901")

	if keyA == keyB {
		t.Fatalf("corretoras distintas produziram a mesma chave de cache: %s", keyA)
	}
}
