# Smoke test de factibilidade do desafio

Este documento não é para o aluno. É a prova, para quem escreve o enunciado, de que o cenário
montado **ensina o que promete ensinar** — antes de gastar uma linha pedindo circuit breaker, cache e
fallback em cima dele.

São duas afirmações, e nenhuma delas pode ficar no "deve funcionar":

1. sem circuit breaker, a plataforma degrada de forma **visível e reprodutível**;
2. os **parâmetros default** dos mocks permitem que um circuit breaker de fato **abra**.

A segunda é a que mata o desafio se estiver errada, e é o risco já registrado na spec: uma
`partner-flaky` instável demais para parecer saudável, mas com as falhas espalhadas de forma tão
regular que nenhum breaker jamais dispara. O aluno passaria a noite depurando um breaker correto.

Executado em **30/07/2026**, em WSL2 com 2 vCPUs e 3 GiB. Ambiente derrubado (`make down`) antes de
cada execução — o contador de sequência dos mocks vive em memória, e começar sujo invalida a
comparação.

---

## 1. Sem circuit breaker, a plataforma degrada — e degrada igual

Duas execuções de `make reproduce`, cada uma a partir do ambiente zerado:

| | baseline (1 em voo) | carga (50 em voo) | p95 |
|---|---|---|---|
| **Execução 1** | 5/10 sucesso · p50 1,89s · p95 2,04s | 120/200 sucesso · p50 6,11s · p95 8,01s | **3,9x** |
| **Execução 2** | 5/10 sucesso · p50 1,89s · p95 2,04s | 120/200 sucesso · p50 5,90s · p95 8,02s | **3,9x** |

O que interessa não é o valor, é a distância entre as duas linhas:

- **A contagem de falhas é idêntica**: 5 no baseline e 80 na carga, nas duas execuções. Não é sorte —
  é a semente. A sequência de falhas da `partner-flaky` é a mesma em toda máquina, sempre.
- **O p95 sob carga variou 10ms** entre execuções (8,01s e 8,02s), porque a `partner-degrading` satura
  no teto de 6s configurado: acima de ~40 requisições em voo o número para de se mexer. É o que torna
  a evidência comparável entre um aluno com notebook rápido e outro com máquina modesta.
- **O p50 é a única coisa que oscila de verdade** (6,11s contra 5,90s): ele mede o meio da fila, e a
  fila depende do escalonamento. Por isso o roteiro pede p95, não média.

A degradação é visível sem ferramenta nenhuma — está no relatório impresso — e o roteiro
(`docs/roteiro-cenario-de-falha.md`) mostra a cascata em série no Jaeger, que é a *causa* dela.

**Critério 1: comprovado.**

---

## 2. Um circuit breaker de fato abre com os defaults

Três provas, do laboratório para a realidade.

### 2.1 A prova determinística — `make smoke`

`cmd/partner-mock/feasibility_test.go` simula um breaker de três estados contra o `Behavior` real do
mock, com quatro políticas de disparo que qualquer biblioteca do mercado oferece (gobreaker,
resilience4j, Polly). O detalhe que torna a simulação honesta: **enquanto o circuito está aberto, o
parceiro não é chamado** — e o número de sequência dele não avança. Uma simulação que lesse a
sequência de ponta a ponta creditaria ao breaker falhas que ele nunca viu.

Em 210 requisições, que é exatamente o que um `make reproduce` envia (10 de baseline + 200 de carga):

| Política de disparo | Abre na requisição | Aberturas | Voltas a fechado |
|---|---|---|---|
| 3 falhas consecutivas | 9 | 13 | 4 |
| 5 falhas consecutivas | 42 | 7 | 2 |
| 50% de falha em janela de 10 | 10 | 15 | 6 |
| 60% de falha em janela de 20 | 55 | 8 | 1 |

Duas leituras, e a segunda costuma passar batido:

- **Todas abrem, e abrem cedo.** Na política mais conservadora da tabela, o breaker abre na
  requisição 55 — dentro da primeira execução do aluno, não depois de uma noite de carga.
- **Todas voltam a fechar.** Um parceiro que falhasse para sempre seria tão inútil quanto um que
  nunca falha: o estado *half-open* só significa alguma coisa se existir sucesso do outro lado do
  cooldown. Com 40% de falha o breaker oscila — abre, prova, fecha, abre de novo. É o comportamento
  que dá o que medir na entrega 2.

O teste `TestFeasibilityBurstIsWhereTheDocsSayItIs` prende o número citado no `docker-compose.yml`: a
rajada de **9 falhas consecutivas nas sequências 49 a 57**. Mexer na semente ou na taxa de falha sem
refazer este smoke test quebra o teste com a mensagem certa.

### 2.2 A prova end-to-end — o mock como ele sobe no compose

O teste acima exercita o `Behavior` em processo. Falta saber se o binário, dentro do container, com a
configuração do `docker-compose.yml`, produz a mesma coisa. 60 requisições sequenciais direto na
`partner-flaky` (porta 9002), ambiente recém-subido:

```
  1 ...XX.XXX..X..X.X...XXX.....X..X.....XXXXX......XX
 51 XXXXXXX.X.
```

(`X` = 503, `.` = 200, numerado pelo header `X-Partner-Seq`.)

Casa **caractere por caractere** com a sequência prevista pelo teste determinístico. A rajada das
sequências 49 a 57 está lá, inteira: nove 503 seguidos. Qualquer breaker que conte falhas
consecutivas abre ali.

### 2.3 O pior caso — a rajada sobrevive à concorrência?

A ressalva está documentada no próprio `behavior.go`: sob concorrência, **qual** requisição recebe
**qual** número de sequência depende da ordem de chegada. O que se repete é a *sequência de
respostas*, não o par requisição-resposta. Se a concorrência embaralhasse a rajada o bastante, um
breaker de falhas consecutivas poderia nunca disparar.

Primeiro, quanta concorrência a `partner-flaky` realmente vê? Medido com 20 sondas durante a fase de
carga, lendo o header `X-Partner-Inflight`:

```
2 1 1 1 1 1 1 1 1 1 2 34 6 1 1 1 1 1 1 1
```

Quase sempre **1** — a própria sonda. Faz sentido: a agregação é em série, e a `partner-flaky` ocupa
~175ms de uma requisição de ~8s. Com 50 em voo na porta da API, quase nunca há duas dentro dela ao
mesmo tempo. As exceções são as **ondas**: nas viradas de rodada as requisições chegam em bloco na
parceira lenta e saem dela juntas, e aí a `partner-flaky` leva 34 de uma vez.

O pior caso, então, é essa onda — e ele foi medido: 210 requisições na `partner-flaky` com **34
simultâneas fixas**, registrando o resultado em **ordem de conclusão**, que é a ordem em que um
breaker dentro da `quotation-api` registraria:

| | Sequencial | 34 simultâneas |
|---|---|---|
| Taxa de falha | 40% | 40,5% |
| Maior rajada | 9 | **5** |
| Breaker de 3 consecutivas abre em | 9 | **3** |
| Breaker de 5 consecutivas abre em | 42 | **104** |
| Pior janela de 20 | 70% | **60%** |

A concorrência **embaralha** a rajada (de 9 para 5) mas não a dissolve, e as quatro políticas
continuam abrindo dentro da primeira execução. O caso realista — concorrência 1, seção 2.2 — é ainda
mais favorável que este.

**Critério 2: comprovado.**

---

## 3. O que fica travado por teste

O smoke test não é um ritual de uma vez só. O que ele comprovou está preso em
`cmd/partner-mock/feasibility_test.go`, que roda em `make test` como qualquer outro teste:

- se alguém mexer em `PARTNER_SEED`, `PARTNER_FAILURE_RATE` ou no algoritmo de decisão e o breaker
  deixar de abrir, ou abrir tarde demais, o teste falha;
- se a rajada mudar de lugar, o teste falha apontando este documento.

O que **não** dá para travar em teste é a parte empírica: o número de vezes que o p95 se multiplica
depende da máquina. Por isso a seção 1 registra duas execuções em vez de um alvo numérico.

## 4. Como refazer

```bash
make smoke                      # a prova deterministica, sem Docker (segundos)
make down && make reproduce     # a degradacao, do zero (~1 min)
make down && make reproduce     # de novo: os numeros tem que se repetir
```

Para refazer as seções 2.2 e 2.3 é preciso ambiente zerado (`make down && docker compose up -d
--wait`) antes de cada sonda — o contador de sequência dos mocks vive em memória, e sondar um
ambiente já usado mede o meio da sequência, não o começo dela.
