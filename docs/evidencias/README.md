# Evidências de Medição e Validação Arquitetural

Este diretório reúne as evidências empíricas coletadas no ambiente local antes e depois da intervenção arquitetural, comprovando a eficácia dos mecanismos de **Circuit Breaker**, **Cache com isolamento de tenant** e **Fallback (resposta parcial)**.

---

## Tabela de Evidências

| Evidência | Comando / Consulta | Janela / Escopo | Artefato | Legenda explicativa |
| :--- | :--- | :--- | :--- | :--- |
| **1. Relatório Antes / Depois** | `make reproduce` (ambiente zerado, 10 baseline + 200 carga @ 50 inflight) | 210 requisições (UTC 16:31 antes, UTC 16:44 depois) | [relatorio-antes.txt](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/relatorio-antes.txt)<br/>[relatorio-depois.txt](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/relatorio-depois.txt) | Sucesso subiu de 60% para 100%, p95 sob carga despencou de 8,00 s para 44 ms (181x mais rápido) e vazão saltou de 8,0 para 3.612,3 req/s. |
| **2. Saída do Test Suite** | `go test -v ./...` | Suíte completa (testes pré-existentes + novos testes de resiliência) | [saida-testes.txt](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/saida-testes.txt) | 100% dos testes unitários e de factibilidade passando, incluindo testes determinísticos de abertura do breaker e isolamento de cache. |
| **3. Trace com Breaker Aberto / Fallback** | `GET http://localhost:16686/api/traces/87a9c6b2e193fa73f089c40e490432c2` | Jaeger trace view (`Last Hour`, UTC 16:46:41) | [trace-breaker-aberto.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/trace-breaker-aberto.json) | Requisição com `partner-flaky` curto-circuitada (`partner.circuit_breaker.state=open`, `partner.short_circuited=true`), sem span de saída para `partner-flaky` e com `partner-slow` (~1,53 s) e `partner-degrading` (~128 ms) respondendo 200 OK degradado. |
| **4. Trace Servido de Cache** | `GET http://localhost:16686/api/traces/cc4e89c7cf63d219b916bdf361718108` | Jaeger trace view (`Last Hour`, UTC 16:44:20) | [trace-cache-hit.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/trace-cache-hit.json) | Cotação servida diretamente do Redis durante a carga com `quotation.cache_hit=true` e `tenant=corretora-a` em 114 µs (0,11 ms) sem emissão de spans HTTP externos. |
| **5. Estado do Breaker no Tempo** | `quotation_circuit_breaker_state` (`GET /api/v1/query_range?query=quotation_circuit_breaker_state&start=1794847567&end=1794847627&step=5s`) | Janela da execução (`16:46:07` a `16:47:07 UTC`, `step=5s`) no Prometheus | [prometheus-breaker-state.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-breaker-state.json) | Série temporal (`matrix`) expondo o degrau completo de transições da `partner-flaky`: fechado (`0`) → aberto (`2`) → meio-aberto (`1`) → fechado (`0`). |
| **6. Hit Rate do Cache no Tempo** | `sum(increase(quotation_cache_requests_total{result="hit"}[5m])) / sum(increase(quotation_cache_requests_total[5m]))` (`GET /api/v1/query_range?...&start=1794847456&end=1794847486&step=5s`) | Janela `[5m]` durante a execução (`16:44:16` a `16:44:46 UTC`, `step=5s`) no Prometheus | [prometheus-cache-hit-rate.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-cache-hit-rate.json)<br/>[prometheus-cache-requests.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-cache-requests.json) | Série temporal (`matrix`) capturada durante a carga: hit rate parte de `0` (0% nos misses iniciais do baseline) e sobe para `0,9716` (97,2%, com 205 hits acumulados em `corretora-a`) conforme o cache aquece. |
| **7. p95 do POST /quotes no Tempo (Antes vs Depois)** | `histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[5m])))` (`GET /api/v1/query_range?...&step=5s`, mesma escala em segundos) | Janela `[5m]` durante cada execução (`16:31:15`–`16:31:55 UTC` antes; `16:44:16`–`16:44:46 UTC` depois, `step=5s`) | [prometheus-p95-antes.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-p95-antes.json)<br/>[prometheus-p95-depois.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-p95-depois.json) | Séries temporais (`matrix`) na mesma escala (segundos): no "antes" o p95 parte de `2,425 s` no baseline e degrada até `9,698 s` sob carga; no "depois" parte de `2,425 s` nos misses iniciais e despenca para `0,0474 s` (`47,4 ms`, alinhado aos `44 ms` do loadgen). |

---

## Síntese Quantitativa: Antes vs Depois

```text
========================================================================================
MÉTRICA                    | ANTES (Sem Proteção)         | DEPOIS (Com SAD & PoC)
========================================================================================
Taxa de Sucesso (Carga)    | 60% (120/200, 80 erros 502)  | 100% (200/200, 0 erros)
Latência p50 (Carga)       | 6,11 s                       | 1 ms
Latência p95 (Carga)       | 8,00 s (9,70 s Prometheus)   | 44 ms (47,4 ms Prometheus)
Latência Máxima (Carga)    | 8,05 s                       | 53 ms
Vazão Efetiva (Carga)      | 8,0 req/s                    | 3.612,3 req/s
Consultas Compradas (Carga)| 360 chamadas externas        | 15 chamadas externas (-95,8%)
Comportamento em Falha     | Abortava requisição inteira  | Fallback parcial transparente
========================================================================================
```
