GO      ?= go
COMPOSE ?= docker compose
BINDIR  ?= bin

.DEFAULT_GOAL := help

## --- Ambiente (Docker Compose) ---

.PHONY: up
up: ## Sobe o ambiente completo em background (recompila as imagens do starter)
	$(COMPOSE) up -d --build

.PHONY: down
down: ## Derruba o ambiente e remove os volumes
	$(COMPOSE) down -v

.PHONY: ps
ps: ## Mostra o estado dos servicos
	$(COMPOSE) ps

.PHONY: logs
logs: ## Acompanha os logs do ambiente
	$(COMPOSE) logs -f

## --- Cenario de falha ---

# O comando unico do desafio: sobe o ambiente, espera ficar saudavel e roda a carga. `--wait` e o que
# torna isso confiavel — sem ele a carga comecaria contra uma API que ainda esta subindo, e o
# relatorio mediria o boot, nao a degradacao.
.PHONY: reproduce
reproduce: ## Sobe o ambiente e reproduz o cenario de falha de ponta a ponta
	$(COMPOSE) up -d --build --wait
	$(MAKE) load

.PHONY: load
load: ## Roda o gerador de carga no ambiente ja de pe (ARGS="-concurrency 80" para variar)
	$(COMPOSE) run --rm --build loadgen $(ARGS)

# Comprova que os defaults dos mocks permitem que um circuit breaker de fato abra. E deterministico
# (nao precisa de Docker nem do ambiente de pe): mesma semente, mesma sequencia, mesmo veredito.
.PHONY: smoke
smoke: ## Roda o smoke test de factibilidade do desafio
	$(GO) test ./cmd/partner-mock -run Feasibility -v

## --- Aplicacao (Go) ---

.PHONY: build
build: ## Compila os binarios de cmd/ em bin/
	$(GO) build -o $(BINDIR)/ ./cmd/...

.PHONY: run
run: ## Roda a quotation-api localmente (PORT=8080 por padrao)
	$(GO) run ./cmd/quotation-api

.PHONY: test
test: ## Roda os testes
	$(GO) test ./...

.PHONY: fmt
fmt: ## Formata o codigo
	$(GO) fmt ./...

.PHONY: vet
vet: ## Roda a analise estatica do Go
	$(GO) vet ./...

.PHONY: tidy
tidy: ## Sincroniza go.mod e go.sum
	$(GO) mod tidy

.PHONY: clean
clean: ## Remove os binarios compilados
	rm -rf $(BINDIR)

## --- Ajuda ---

.PHONY: help
help: ## Lista os comandos disponiveis
	@awk 'BEGIN {FS = ":.*?## "} \
		/^## ---/ { gsub(/^## /, ""); printf "\n%s\n", $$0; next } \
		/^[a-zA-Z_-]+:.*?## / { printf "  %-10s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""
