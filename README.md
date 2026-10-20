# Prumo Cota — Resiliência, Cache Multi-Tenant e Observabilidade de Fronteira

Este repositório contém a entrega completa do desafio **Fundamentos de Arquitetura de Solução** do MBA em Arquitetura de Soluções da Full Cycle. O projeto transforma uma plataforma de cotação de seguros automotivos fragilizada — caracterizada por consultas sequenciais lentas, ausência de timeout e falhas em cascata tudo-ou-nada — em uma arquitetura de alta disponibilidade, proteção financeira e conformidade com SUSEP e LGPD.

---

### O Coração da Solução

> Protegemos a margem financeira e a estabilidade da Prumo Tecnologia blindando a fronteira de integração com os motores de precificação legados das seguradoras. Introduzimos **Circuit Breakers dedicados por parceira** (utilizando a biblioteca Sony `gobreaker/v2` com limiar de 5 falhas consecutivas, janela de 10s e cooldown de 5s) e **timeout rígido de 2.500 ms**, garantindo que a instabilidade da `partner-flaky` e o afundamento por concorrência da `partner-degrading` sejam curto-circuitados em 0 ms sem emissão de tráfego HTTP. Acoplamos uma camada de **Cache Redis com isolamento estrito de inquilino** via chave canônica literal `quote:v1:{tenant}:{coverage}:{plate}:{sha256(document)}` com TTL de 300 segundos, eliminando o custo de R$ 0,04 por requisição em recotações e pseudonimizando dados pessoais nos termos da LGPD. Em situações de falha externa, ativamos o **Fallback por resposta parcial enriquecida**, entregando à corretora as cotações das parceiras disponíveis ordenadas por menor preço com HTTP 200 OK degradado. A solução custou R$ 116.000 em esforço de engenharia (CAPEX) e R$ 3.180/mês em infraestrutura adicional, gerando uma economia bruta mensal de **R$ 95.040 a R$ 158.400**, com **payback completo em apenas 41 dias**.

---

## 1. Links para os Documentos e Artefatos de Entrega

- **Entrega 1 — Solution Architecture Document (SAD):** [docs/sad.md](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/sad.md) (todas as 8 seções completas, 4 diagramas C4 em Mermaid e demonstração analítica de TCO/ROI).
- **Catálogo de Evidências Medidas:** [docs/evidencias/README.md](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/README.md)
  - [Relatório Antes (Sem Proteção)](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/relatorio-antes.txt) vs [Relatório Depois (Com Resiliência)](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/relatorio-depois.txt)
  - [Saída Completa do Test Suite](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/saida-testes.txt)
  - [Trace Jaeger com Resposta Servida de Cache](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/trace-cache-hit.json)
  - [Trace Jaeger com Falha / Circuit Breaker Aberto](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/trace-breaker-aberto.json)
  - [Métricas Prometheus — Estado do Breaker](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-breaker-state.json)
  - [Métricas Prometheus — Requisições de Cache e Hit Rate](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-cache-requests.json)
- **Enunciado Original Preservado:** [docs/enunciado.md](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/enunciado.md)

---

## 2. Como Subir o Ambiente e Reproduzir

### Pré-requisitos
- Docker com Docker Compose v2 ativo;
- Go 1.25+ (opcional para testes locais).

### Comandos de Execução
```bash
# 1. Clonar o repositório e entrar no diretório
cd mba_arquitetura_desafio_cotacao_seguros

# 2. Subir o ambiente completo e executar a carga padronizada (make reproduce)
# (10 requisições sequenciais de baseline + 200 requisições com 50 simultâneas)
make reproduce

# 3. Rodar os testes determinísticos automatizados
make test

# 4. Rodar o smoke test de factibilidade
make smoke
```

### URLs dos Serviços no Ambiente Local
- **Quotation API:** `http://localhost:8080` (Healthcheck: `http://localhost:8080/healthz`)
- **Jaeger UI (Traces):** `http://localhost:16686` (Serviço `quotation-api`, operação `POST /quotes`)
- **Prometheus (Métricas):** `http://localhost:9090`
- **Redis Cache:** `localhost:6379`
- **Mocks das Parceiras:** `localhost:9001` (slow), `localhost:9002` (flaky), `localhost:9003` (degrading)

---

## 3. O que Foi Implementado e o que Ficou como Proposta

### Implementado no PoC (Código em Execução e Provado)
1. **Circuit Breaker Dedicado por Parceira:** Implementado em [internal/partner/breaker.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/breaker.go) e integrado a [internal/partner/client.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/client.go), utilizando Sony `gobreaker/v2`. Em estado `Open`, curto-circuita em 0 ms e não dispara requisição HTTP externa;
2. **Timeout Limítrofe de 2.500 ms:** Configurado em `Client.timeout` com cancelamento via contexto. Estouro de timeout é registrado e conta no contador de falhas do breaker, forçando a abertura caso a `partner-degrading` afunde;
3. **Cache Redis com Isolamento Multi-Tenant:** Implementado em [internal/cache/redis.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/cache/redis.go), com chave canônica `quote:v1:{tenant}:{coverage}:{plate}:{sha256(document)}`, TTL padrão de 300 segundos e tratamento *fail-open*;
4. **Fallback com Resposta Parcial Enriquecida:** Implementado em [internal/quotation/service.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/service.go) e [internal/quotation/request.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/request.go), devolvendo HTTP 200 OK com `degraded: true`, lista `unavailable_partners` e propostas ordenadas;
5. **Agregação Paralela Concorrente (Bônus):** Execução concorrente das consultas externas via goroutines, reduzindo a latência da agregação de $\sum T_i$ para $\max(T_i)$;
6. **Instrumentação de Métricas de Negócio e Traces:** Registradas em [internal/telemetry/metrics.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry/metrics.go);
7. **Suíte Completa de Testes Determinísticos:** Testes de transição de breaker sem sleeps arbitrários em [internal/partner/breaker_test.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/breaker_test.go) e de cache/fallback em [internal/quotation/resilience_test.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/resilience_test.go).

### O que Ficou como Proposta de Arquitetura (Documentado no SAD)
1. **Bulkhead com Semáforo / Concorrência Máxima por Parceira:** Proposto na Seção 4 do SAD para limitar fisicamente em 5 as chamadas simultâneas à `partner-degrading`, impedindo que ela entre em sobrecarga;
2. **Circuit Breaker Distribuído em Camada Compartilhada:** Proposto para unificar contadores de falhas em clusters com dezenas de nós escalonados horizontalmente;
3. **Mecanismo Ativo de Invalidação de Cache por Webhooks:** Proposto para permitir que seguradoras notifiquem alterações atuariais de tabelas de preço antes da expiração natural do TTL;
4. **Armazenamento de Longo Prazo em S3 Glacier WORM:** Modelado financeiramente na Seção 8 do SAD para arquivamento regulatório imutável de 5 anos exigido pela SUSEP.

---

## 4. Nomes das Métricas e Atributos Criados

### Métricas de Negócio (Prometheus / OpenTelemetry)

| Nome da Métrica | Tipo | Descrição e Valores | Rótulos / Atributos Permitidos |
| :--- | :--- | :--- | :--- |
| `quotation_circuit_breaker_state` | Gauge (Observable) | Expõe o estado operacional do breaker por parceira: `0` = Closed, `1` = Half-Open, `2` = Open | `partner` (ex: `partner-flaky`, `partner-slow`, `partner-degrading`) |
| `quotation_circuit_breaker_transitions_total` | Counter | Contabiliza as mudanças de estado do circuit breaker | `partner`, `from` (estado origem), `to` (estado destino) |
| `quotation_cache_requests_total` | Counter | Registra acessos ao cache e permite calcular o hit rate | `result` (`hit` ou `miss`), `tenant_id` (ex: `corretora-a`) |
| `quotation_partner_request_duration_seconds` | Histogram | Latência de chamada à seguradora parceira com significado de domínio | `partner`, `status` (`200`, `503`, `timeout`, `error`) |

### Marcações em Traces (Spans no Jaeger)

- `quotation.cache_hit`: Atributo booleano (`true` ou `false`) gravado no span `POST /quotes`, comprovando resposta servida do Redis;
- `tenant`: Identificador da corretora/inquilino (ex: `corretora-a`), permitindo correlação multi-tenant sem expor PII ou placa;
- `partner.circuit_breaker.state`: Atributo de string gravado como `"open"` no span quando a chamada é curto-circuitada;
- `partner.short_circuited`: Atributo booleano (`true`) confirmando que nenhuma chamada de rede foi emitida à seguradora.

> **Garantia de Privacidade LGPD:** Nenhum CPF, placa de veículo ou identificador pessoal foi incluído como atributo de span ou rótulo de métrica, preservando a cardinalidade controlada e evitando vazamento de dados em telemetria.

---

## 5. O que Faria Diferente com Mais Tempo

1. **Implementação de Bulkhead com Leaky Bucket:** Desenvolveria um semáforo de concorrência adaptativo dedicado para a `partner-degrading` (capacidade máxima de 5 conexões simultâneas), enfileirando requisições excedentes por até 200 ms antes de rejeitar com fail-fast, impedindo que ela entre na zona de degradação;
2. **Circuit Breaker Distribuído com Redis Sliding Window:** Implementaria contadores de falhas descentralizados com Redis, permitindo que todas as réplicas da API compartilhassem instantaneamente a percepção de queda de uma seguradora;
3. **Rate Limiting e Quotas por Inquilino (Token Bucket por Tenant):** Adicionaria controle de vazão granular por cabeçalho `X-Tenant-Id`, garantindo isolamento de capacidade contra o problema do "vizinho barulhento" (*noisy neighbor*);
4. **Pipeline Assíncrono de Ingestão de Auditoria SUSEP:** Construiria um worker desacoplado enviando eventos de cotações apresentadas para um bucket S3 com Object Lock WORM em lotes Parquet compactados, reduzindo ainda mais o custo de retenção regulatória de 5 anos;
5. **Dashboard Unificado em Grafana:** Exportaria um painel provisionado como código em JSON reunindo em tempo real os gauges dos breakers, curva de aquecimento do cache e histogramas comparativos de latência.
