# Roteiro: reproduzir e observar o cenário de falha

Este roteiro é o **"antes"** do desafio. Ele mostra, em um comando, a plataforma de cotação
degradando sob carga — e ensina a encontrar no Jaeger *por que* ela degrada. O "depois" (com
circuit breaker, cache e fallback) é o que você vai construir e medir do mesmo jeito.

Nada aqui pede código seu. Rode, leia, e só então decida o que arquitetar.

---

## 1. O comando único

```bash
make reproduce
```

Ele sobe o ambiente completo (`docker compose up -d --build --wait`, esperando cada serviço ficar
saudável) e roda o gerador de carga dentro da rede do Compose. Na primeira vez as imagens são
compiladas — conte com cerca de um minuto a mais.

Variações:

| Situação | Comando |
|---|---|
| Ambiente já de pé | `make load` |
| Ajustar a carga | `make load ARGS="-concurrency 100"` |
| Ver todas as opções | `make load ARGS="-h"` |
| Rodar do host, com Go instalado | `go run ./cmd/loadgen` |

---

## 2. O que o relatório diz

Saída real de uma execução (30/07/2026), com o ambiente recém-subido. **As suas latências vão
diferir** — elas dependem da máquina. As **contagens de falha**, não: elas vêm da semente do
`partner-flaky`, e são as mesmas para todo mundo que rodar a partir de um ambiente zerado.

```
baseline — 10 requests, 1 in flight, 16:17:53 UTC to 16:18:11 UTC
  success        5 of 10 (50%)
  latency        p50 1.89s   p95 2.04s   p99 2.04s   max 2.04s
  throughput     0.5 req/s in 18.79s
  failures       HTTP 502 from partner-flaky: 5

load — 200 requests, 50 in flight, 16:18:11 UTC to 16:18:36 UTC
  success        120 of 200 (60%)
  latency        p50 6.11s   p95 8.01s   p99 8.03s   max 8.05s
  throughput     8.0 req/s in 24.90s
  failures       HTTP 502 from partner-flaky: 80

baseline → load
  p95 latency    2.04s → 8.01s   3.9x
  max latency    2.04s → 8.05s   4.0x
  success        50% → 60%
  throughput     0.5 → 8.0 req/s
```

Como ler:

- **O baseline já é ruim.** Uma requisição por vez, sem concorrência nenhuma, e a cotação leva ~1,9s.
  Esse é o *melhor caso* da plataforma: a soma das três parceiras consultadas em série.
- **A carga multiplica por 4.** Com 50 requisições em voo, o p95 vai a ~8s. Nenhuma parceira caiu:
  a `partner-degrading` só ficou mais lenta, e a agregação síncrona repassou isso inteiro ao cliente.
- **A taxa de erro não é a variável de carga.** Os ~40% de falha vêm da `partner-flaky`, que falha na
  mesma proporção com ou sem carga. Não leia "50% → 60%" como se a carga tivesse melhorado alguma
  coisa: são só 10 requisições contra 200, e numa amostra pequena a proporção balança. O que a carga
  muda é a **latência**; o que a `partner-flaky` mostra é que **uma parceira fora do ar derruba a
  cotação inteira**, porque não há fallback.
- **Rodou de novo e os números de falha mudaram?** Então o ambiente não estava zerado. A sequência de
  falhas é determinística e começa do zero junto com o container: um `make load` em cima de um
  ambiente já usado continua do meio dela. Para comparar duas execuções, `make down` antes de cada
  uma.
- **Throughput sobe, experiência piora.** A plataforma aceita mais requisições por segundo justamente
  porque cada uma fica mais tempo esperando. Vazão não é desempenho.

> **Relógio:** rodando pelo `make load`, o relatório imprime o horário do container (UTC); o Jaeger
> mostra o horário local do seu navegador. Por isso o roteiro abaixo usa "Last Hour" e ordenação por
> duração, em vez de casar horários na mão.

---

## 3. O que observar no Jaeger

Abra <http://localhost:16686>.

**1. Encontre os traces**

- **Service:** `quotation-api`
- **Operation:** `POST /quotes`
- **Lookback:** `Last Hour` · **Limit:** `200`
- **Find Traces**

O gráfico de dispersão no topo é a primeira evidência: os pontos do baseline ficam agrupados perto de
2s e a nuvem da carga sobe para ~8s. O degrau entre os dois grupos é o cenário de falha.

**2. Abra o trace mais longo** (`Sort: Longest First`)

Você verá **4 spans**: um de servidor, `POST /quotes`, e três de cliente, `HTTP POST` — um por
parceira. Identifique cada uma pela tag `url.full` (ou `server.address`) no detalhe do span.

O que olhar na barra de tempo, medido em um trace real de 7,9s:

| Span | Começa em | Duração |
|---|---|---|
| `POST /quotes` (servidor) | 0 ms | 7896 ms |
| `HTTP POST` → `partner-slow` | 0 ms | 1569 ms |
| `HTTP POST` → `partner-flaky` | 1569 ms | 167 ms |
| `HTTP POST` → `partner-degrading` | 1737 ms | 6160 ms |

**Cada span começa exatamente quando o anterior termina.** Não há sobreposição: as três chamadas são
sequenciais, e a latência da cotação é a **soma** das três. Se fossem paralelas, seria o máximo delas
— outra plataforma, com o mesmo código de negócio.

Repare também na `partner-degrading`: em repouso ela responde em ~120ms. Nesse trace, sob carga, ela
levou 6160ms — o teto configurado. Ela nunca falha; ela afunda. É a parceira que engana o
monitoramento ingênuo baseado só em taxa de erro.

**3. Abra um trace com erro** — no campo **Tags**, use `error=true`

Esse trace tem **um span a menos**. A sequência morre no `HTTP POST` para a `partner-flaky`, com
`http.response.status_code = 503`, e o span da `partner-degrading` **não existe**: a requisição foi
abortada antes de chegar nela. O span de servidor termina com 502.

É a lição inteira em uma tela: a `partner-slow` respondeu, o cliente esperou 1,5s por essa resposta —
e jogou tudo fora porque a parceira seguinte falhou. Sem cache, sem fallback e sem resposta parcial,
a disponibilidade da plataforma é o **produto** da disponibilidade das três parceiras.

**4. Compare com um trace do baseline**

Mesma forma, durações menores. É o ponto que costuma passar batido: a carga não criou o problema,
ela só o tornou grande o bastante para ser visto. A cascata em série está lá desde a primeira
requisição.

> As parceiras não aparecem como serviços próprios no Jaeger: os mocks não são instrumentados de
> propósito. Você as enxerga pela borda, como spans cliente da `quotation-api` — que é exatamente a
> visão que se tem de um parceiro real, e é o que torna a instrumentação da sua própria fronteira
> tão importante.

---

## 4. O que observar no Prometheus (opcional)

Abra <http://localhost:9090>. O trace explica *uma* requisição; a métrica mostra o comportamento
agregado. Três consultas suficientes para este roteiro:

Latência p95 **por parceira** — separa quem é lento de quem é instável:

```promql
histogram_quantile(0.95, sum by (server_address, le) (rate(http_client_request_duration_seconds_bucket[1m])))
```

Requisições por status na parceira instável — os 503 que viram 502 na ponta:

```promql
sum by (http_response_status_code) (rate(http_client_request_duration_seconds_count{server_address="partner-flaky"}[1m]))
```

Latência p95 da própria API, do ponto de vista da corretora:

```promql
histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[1m])))
```

Essas métricas são **genéricas** (HTTP de entrada, HTTP de saída, runtime do Go) e já vêm prontas no
starter. As métricas de **negócio** — estado do circuit breaker, `hit`/`miss` do cache, latência por
parceira com significado de domínio — são sua parte do trabalho.

---

## 5. Variações que vale rodar

| Comando | O que ele mostra |
|---|---|
| `make load ARGS="-concurrency 10"` | Abaixo do joelho a plataforma quase não piora — com 20 em voo o p95 sobe só 1,1x. Degradação por carga tem limiar; entre 25 e 30 ela dispara. |
| `make load ARGS="-concurrency 100"` | A partir de ~40 a `partner-degrading` satura no teto de 6s: mais carga não piora a latência de cada requisição, só a fila. |
| `make load ARGS="-distinct 1"` | Sempre a mesma cotação. É o melhor caso possível para o cache que você vai construir — e hoje não muda absolutamente nada, porque não há cache. |
| `make load ARGS="-timeout 3s"` | O cliente desiste antes da plataforma responder. Quantas cotações sobram? Timeout é a proteção mais barata que falta aqui. |
| `curl -s localhost:9003/config` | A configuração efetiva da `partner-degrading` (limiar, passo e teto da degradação). |
| `docker compose logs partner-degrading` | Os cabeçalhos `X-Partner-*` do mock: sequência, latência aplicada e requisições simultâneas. |

---

## 6. Por que degrada — as três causas, no código

Tudo o que você observou vem de três decisões deliberadamente ingênuas do starter, e nenhuma delas
está escondida:

1. **Agregação síncrona e em série** — `internal/quotation/service.go`. As parceiras são consultadas
   uma após a outra; a latência é a soma.
2. **Nenhum timeout na chamada ao parceiro** — `internal/partner/client.go`. O `http.Client` tem
   `Timeout` zero: a API espera o tempo que a parceira quiser.
3. **Tudo ou nada** — a mesma `service.go`. Uma parceira falhando aborta a requisição inteira. Não há
   circuit breaker, não há cache, não há fallback, não há resposta parcial.

Esse vácuo é o enunciado, não um esquecimento.

---

## 7. Guarde as evidências

Antes de tocar em qualquer código, salve:

- a **saída completa** do `make reproduce` (é o "antes" quantitativo);
- um **screenshot do trace lento**, com os três spans em cascata visíveis;
- um **screenshot do trace com erro**, mostrando o span que não chegou a existir.

É contra esses três artefatos que a sua entrega vai ser comparada — inclusive por você, quando rodar
o mesmo `make reproduce` depois do circuit breaker de pé.

Uma advertência sobre essa comparação, porque ela não é ingênua: o `loadgen` é de **laço fechado**,
com um número fixo de requisições em voo (50, no padrão). Quando a sua entrega derrubar a latência,
os mesmos 50 workers passam a disparar mais requisições por segundo, e a `partner-degrading` vai
receber mais chamadas simultâneas do que recebia no "antes". É esperado, então, que ela apareça
**pior** no "depois", e isso não é regressão da sua arquitetura: é o gerador de carga andando mais
rápido porque você o desbloqueou. O que se compara entre as duas execuções é a taxa de sucesso, o p95
da ponta e a vazão, não a latência isolada da parceira que afunda. Se você quiser separar os dois
efeitos, rode também uma variação com `-concurrency` menor e diga no SAD que rodou.
