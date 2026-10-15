# Evidências de Medição e Validação Arquitetural

Este diretório reúne as evidências empíricas coletadas no ambiente local antes e depois da intervenção arquitetural, comprovando a eficácia dos mecanismos de **Circuit Breaker**, **Cache com isolamento de tenant** e **Fallback (resposta parcial)**.

---

## Tabela de Evidências

| Evidência | Comando / Consulta | Janela / Escopo | Artefato | Legenda explicativa |
| :--- | :--- | :--- | :--- | :--- |
| **1. Relatório Antes / Depois** | `make reproduce` (ambiente zerado, 10 baseline + 200 carga @ 50 inflight) | 210 requisições (UTC 11:44 antes, UTC 12:00 depois) | [relatorio-antes.txt](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/relatorio-antes.txt)<br/>[relatorio-depois.txt](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/relatorio-depois.txt) | Sucesso subiu de 60% para 100%, p95 sob carga despencou de 8,01 s para 55 ms (145x mais rápido) e vazão saltou de 8,1 para 3.108 req/s. |
| **2. Saída do Test Suite** | `go test -v ./...` | Suíte completa (testes pré-existentes + novos testes de resiliência) | [saida-testes.txt](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/saida-testes.txt) | 100% dos testes unitários e de factibilidade passando, incluindo testes determinísticos de abertura do breaker e isolamento de cache. |
| **3. Trace com Breaker Aberto / Fallback** | `GET http://localhost:16686/api/traces/85baf84ee58109c1f2c9576e6802ff06` | Jaeger trace view | [trace-breaker-aberto.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/trace-breaker-aberto.json) | Requisição com parceira instável (`partner-flaky`) em falha/breaker aberto devolvendo 200 OK degradado com as parceiras saudáveis. |
| **4. Trace Servido de Cache** | `GET http://localhost:16686/api/traces/13717a6338bb59033f1770c8e88e9074` | Jaeger trace view | [trace-cache-hit.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/trace-cache-hit.json) | Cotação servida diretamente do Redis com atributo `quotation.cache_hit=true` em 0 ms sem emissão de spans HTTP externos. |
| **5. Estado do Breaker no Tempo** | `quotation_circuit_breaker_state` | Last 5 minutes (`[5m]`) no Prometheus | [prometheus-breaker-state.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-breaker-state.json) | Série temporal observável expondo o estado de cada uma das 3 parceiras (0=fechado, 1=meio-aberto, 2=aberto). |
| **6. Hit Rate do Cache** | `sum(increase(quotation_cache_requests_total{result="hit"}[5m])) / sum(increase(quotation_cache_requests_total[5m]))` | Janela de 5 minutos no Prometheus | [prometheus-cache-requests.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-cache-requests.json)<br/>[prometheus-cache-hit-rate.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-cache-hit-rate.json) | Contador registrando 205 hits e 30 misses com tenant `corretora-a`, comprovando o aquecimento acelerado do cache sob carga. |
| **7. p95 do POST /quotes** | `histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[5m])))` | Janela de 5 minutos no Prometheus antes e depois | [prometheus-p95-antes.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-p95-antes.json)<br/>[prometheus-p95-depois.json](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docs/evidencias/prometheus-p95-depois.json) | Redução drástica da latência p95 na borda HTTP da API, caindo de 9,69 s (antes) para 55 ms (depois no relatório de carga). |

---

## Síntese Quantitativa: Antes vs Depois

```text
========================================================================================
MÉTRICA                    | ANTES (Sem Proteção)         | DEPOIS (Com SAD & PoC)
========================================================================================
Taxa de Sucesso (Carga)    | 60% (120/200, 80 erros 502)  | 100% (200/200, 0 erros)
Latência p50 (Carga)       | 6,11 s                       | 1 ms
Latência p95 (Carga)       | 8,01 s                       | 55 ms (redução de 145x)
Latência Máxima (Carga)    | 8,05 s                       | 60 ms
Vazão Efetiva (Carga)      | 8,1 req/s                    | 3.108,6 req/s
Consultas Compradas (Carga)| 360 chamadas externas        | 30 chamadas externas (-91,6%)
Comportamento em Falha     | Abortava requisição inteira  | Fallback parcial transparente
========================================================================================
```
