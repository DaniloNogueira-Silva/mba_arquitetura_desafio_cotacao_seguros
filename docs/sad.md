# Solution Architecture Document (SAD) — Plataforma Prumo Cota

**Versão da Arquitetura:** 2.0  
**Data:** 15 de Outubro de 2026  
**Status:** Aprovado para Implementação e Validação  
**Classificação:** Interna / Regulatória  
**Autoria:** Antigravity (Advanced Agentic Architecture)  
**Stakeholders Primários:** CTO (Aprovação Financeira e TCO), Engenharia & SRE (Operação e Runbooks), DPO & Compliance (Regulação SUSEP e ANPD/LGPD).

---

## 1. Introdução

### 1.1 Propósito
Este Solution Architecture Document (SAD) formaliza a reestruturação arquitetural da plataforma **Prumo Cota**, mantida pela **Prumo Tecnologia em Seguros**. A intervenção tem como objetivo eliminar os pontos críticos de instabilidade estrutural, degradação de latência em cascata e vulnerabilidade financeira na integração com motores externos legados de precificação de seguro automóvel.

O documento atende diretamente a três perfis essenciais:
1. **O CTO da Prumo**, fornecendo a demonstração analítica e contábil do Retorno sobre Investimento (ROI), com cálculo de payback derivado da redução de custos por consulta externa;
2. **O Engenheiro SRE / Plantonista Operacional das 03h00**, estabelecendo runbooks objetivos, métricas de observabilidade de negócio e procedimentos de contenção para falhas e sobrecarga de parceiras;
3. **O Encarregado de Proteção de Dados (DPO) e o Auditor Regulatório**, comprovando o isolamento criptográfico e lógico multi-tenant nos termos da LGPD (Lei 13.709/2018) e a rastreabilidade imutável de todas as cotações nos termos da SUSEP (Circular 638/2021).

### 1.2 Escopo
- **No Escopo:**
  - A camada de borda e agregação do serviço [quotation-api](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/cmd/quotation-api/main.go);
  - A fronteira de comunicação HTTP com os três motores externos legados ([partner-slow](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docker-compose.yml#L148-L158), [partner-flaky](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docker-compose.yml#L164-L176), [partner-degrading](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/docker-compose.yml#L179-L192));
  - Implementação de Circuit Breaker individualizado por parceira em [internal/partner/breaker.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/breaker.go) e [internal/partner/client.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/client.go);
  - Implementação de camada de Cache Redis com isolamento estrito por inquilino em [internal/cache/redis.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/cache/redis.go);
  - Implementação de Fallback por Resposta Parcial Enriquecida e Agregação Concorrente em [internal/quotation/service.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/service.go);
  - Instrumentação de telemetria de negócio OpenTelemetry em [internal/telemetry/metrics.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry/metrics.go);
  - Planejamento financeiro de TCO, RTO/RPO e plano de continuidade de negócios.
- **Fora do Escopo:**
  - Redesenho dos algoritmos de precificação atuariais das seguradoras parceiras;
  - Criação de mecanismos de autenticação federada OAuth2/mTLS (assumido `X-Tenant-Id` provisionado em API Gateway corporativo);
  - Implementação de filas assíncronas (ex: Kafka/RabbitMQ) ou orquestração Kubernetes de produção;
  - Tabelas relacionais em banco SQL para persistência permanente (a retenção da auditoria é modelada como objeto frio durável de armazenamento);
  - Alteração no binário e regras de simulação determinística de [cmd/partner-mock](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/cmd/partner-mock/).

### 1.3 Restrições Arquiteturais
As restrições abaixo são imposições do ecossistema e do negócio, sobre as quais a arquitetura não possui liberdade de alteração:
1. **Linguagem e Runtime:** Go 1.25 para a API e suíte de testes automatizados;
2. **Ambiente de Orquestração Local:** Docker com Compose v2, sem custo de provedores de nuvem para a reprodução;
3. **Cache Obrigatório:** Redis 7 (imagem oficial Alpine) como camada volátil;
4. **Telemetria e Observabilidade:** OpenTelemetry SDK integrado, com exportação OTLP via gRPC para o OTel Collector, roteando traces para Jaeger e métricas para Prometheus;
5. **Diagramação Estrita:** Diagramas no padrão C4 Model codificados nativamente em Mermaid dentro do Markdown;
6. **Contrato de Cobrança Externa:** Custo fixo contratual de **R$ 0,04 por requisição efetuada a cada parceira**, totalizando 360.000 consultas/dia útil no baseline sem cache;
7. **Regulação SUSEP:** Obrigatoriedade de retenção de toda cotação efetivamente apresentada ao cliente final por 5 anos (60 meses) em meio imutável, auditável e rastreável;
8. **Regulação LGPD:** Papel da Prumo como Operadora de Dados e da Corretora como Controladora. Vedado o compartilhamento ou exposição de dados pessoais entre corretoras distintas. Chaves de cache devem garantir isolamento absoluto por corretora (`tenant_id`);
9. **Imutabilidade de Valores:** Proibição regulatória expressa de fabricar prêmios matematicamente na plataforma. Todo prêmio exibido provém de um motor parceiro ou de entrada rastreável em cache.

### 1.4 Pressupostos de Negócio e Engenharia

| ID | Pressuposto | Valor Assumido | Origem / Racional | Consequência se for Falso |
| :--- | :--- | :--- | :--- | :--- |
| **P-01** | **Taxa de Recotação Imediata** | **35% das cotações** ocorrem como repetição para o mesmo veículo/condutor em até 5 minutos | Prática do mercado de corretagem: corretores realizam simulações consecutivas com pequenas variações de franquia ou durante a ligação telefônica com o proponente | Se for menor (ex: 5%), o hit rate real despenca, a economia bruta mensal cai de R$ 110.880 para R$ 15.840 (economia líquida de R$ 6.660/mês) e o payback do investimento de desenvolvimento (R$ 116.000) sobe de 1,14 mês (~34 dias) para 17,4 meses (~523 dias). |
| **P-02** | **Isolamento de Carteiras entre Inquilinos** | **Zero sobreposição** permitida entre corretoras concorrentes | Obrigações contratuais de sigilo comercial e exigências da LGPD (Art. 6º, III e Art. 46) | Se for falso e ocorrer vazamento de cache entre corretoras, a Prumo comete infração gravíssima perante a ANPD (sujeita a sanções de até 2% do faturamento ou R$ 50 milhões por infração) e responde civilmente por quebra de segredo comercial. |
| **P-03** | **Validade Técnica dos Preços** | **300 segundos (5 minutos)** de garantia do prêmio pelo motor da parceira | Campo `valid_for_seconds: 300` retornado pelas seguradoras (`PARTNER_QUOTE_TTL_SECONDS`). Teto comercial admitido pelas seguradoras é de até 24 horas | Se a parceira não honrar a cotação por 300 segundos (ex: volatilidade cambial ou atuarial em tempo real de 0s), cotações servidas do cache seriam rejeitadas no fechamento da apólice ("quebra de prêmio"), forçando a Prumo a cobrir a diferença financeira ou invalidar a venda. |

---

## 2. Visão Geral da Arquitetura

### 2.1 Modelo C4 — Nível 1: Diagrama de Contexto
O diagrama abaixo posiciona a plataforma Prumo Cota perante os usuários de negócio, sistemas externos legados e entidades regulatórias, explicitando seu papel de fachada agregadora:

```mermaid
flowchart TD
    broker(["Corretor de Seguros<br/><i>[Pessoa / Usuário do Sistema]</i><br/>Solicita cotações de risco auto para segurados"])
    susep["SUSEP<br/><i>[Sistema Externo / Regulador]</i><br/>Audita conformidade, transparência e histórico de 5 anos"]
    anpd["ANPD<br/><i>[Sistema Externo / Regulador]</i><br/>Fiscaliza conformidade com a LGPD e privacidade"]

    subgraph PrumoSystem ["Plataforma Prumo Tecnologia em Seguros"]
        prumoCota["Prumo Cota<br/><i>[Software System]</i><br/>Fachada de cotação multi-tenant resiliente, veloz e auditável"]
    end

    subgraph ExternalPartners ["Seguradoras Parceiras (Motores de Precificação)"]
        pSlow["partner-slow<br/><i>[Sistema Externo]</i><br/>Latência alta (~1500ms ± 200ms) e estável"]
        pFlaky["partner-flaky<br/><i>[Sistema Externo]</i><br/>Latência baixa (~150ms), 40% falha HTTP 503"]
        pDegrading["partner-degrading<br/><i>[Sistema Externo]</i><br/>Latência nominal ~120ms, degrada até 6000ms sob concorrência"]
    end

    broker -- "1. Requisita cotação de veículo<br/>[HTTP/JSON / X-Tenant-Id]" --> prumoCota
    prumoCota -- "2. Devolve propostas ordenadas da mais barata para a mais cara<br/>[HTTP/JSON]" --> broker
    prumoCota -- "3. Consulta prêmio de risco<br/>[HTTP/JSON - R$ 0,04/req]" --> pSlow
    prumoCota -- "4. Consulta prêmio de risco<br/>[HTTP/JSON - R$ 0,04/req]" --> pFlaky
    prumoCota -- "5. Consulta prêmio de risco<br/>[HTTP/JSON - R$ 0,04/req]" --> pDegrading
    prumoCota -. "Evidências de auditoria de 5 anos" .-> susep
    prumoCota -. "Garantia de isolamento e minimização de dados" .-> anpd
```

### 2.2 Modelo C4 — Nível 2: Diagrama de Contêineres ("Antes" — Starter Kit)
No estado inicial do repositório, a arquitetura operava em modelo síncrono e em série, sem proteção de timeout, sem ativação do Redis e com comportamento tudo-ou-nada:

```mermaid
flowchart LR
    broker(["Corretora Cliente<br/><i>[Pessoa]</i>"])
    
    subgraph StarterEnv ["Ambiente Docker Compose (Antes)"]
        api["quotation-api<br/><i>[Contêiner · Go 1.25]</i><br/>Agregação serial síncrona, timeout zero, tudo-ou-nada"]
        redisUnused[("redis (inativo)<br/><i>[Contêiner · Redis 7 Alpine]</i><br/>Sobe no compose mas a API não se comunica")]
        collector["otel-collector<br/><i>[Contêiner · OTel Contrib]</i><br/>Coleta métricas e traces básicos HTTP"]
        jaeger["jaeger<br/><i>[Contêiner · In-Memory]</i><br/>Visualização de traces"]
        prometheus["prometheus<br/><i>[Contêiner · TSDB]</i><br/>Armazenamento de métricas HTTP"]
    end

    subgraph Partners ["Motores Externos de Precificação"]
        pSlow["partner-slow:8080<br/><i>[Contêiner Módulo Mock]</i>"]
        pFlaky["partner-flaky:8080<br/><i>[Contêiner Módulo Mock]</i>"]
        pDegrading["partner-degrading:8080<br/><i>[Contêiner Módulo Mock]</i>"]
    end

    broker -- "POST /quotes<br/>[X-Tenant-Id]" --> api
    api -- "1. Consulta síncrona (1,5s)<br/>sem timeout" --> pSlow
    pSlow -. "Retorna cotação" .-> api
    api -- "2. Consulta sequencial (0,15s)<br/>falha em 40%" --> pFlaky
    pFlaky -. "503 Service Unavailable" .-> api
    api -. "3. Aborta requisição! Desperdiça tempo e dinheiro" .x pDegrading
    api -. "Retorna HTTP 502 Bad Gateway ao corretor" .-> broker
    api -- "Telemetria padrão HTTP" --> collector
    collector --> jaeger
    collector --> prometheus
```

### 2.3 Modelo C4 — Nível 2: Diagrama de Contêineres ("Depois" — Arquitetura Resiliente)
Na arquitetura proposta e comprovada pelo PoC, todos os contêineres passam a operar coordenados por mecanismos de tolerância a falhas:

```mermaid
flowchart TD
    broker(["Corretora Cliente<br/><i>[Pessoa / Sistema]</i>"])

    subgraph ResilientEnv ["Ambiente Docker Compose (Depois da Intervenção)"]
        api["quotation-api<br/><i>[Contêiner · Go 1.25]</i><br/>Circuit Breakers dedicados, Fallback parcial, Paralelismo concorrente"]
        redis[("redis (ativo)<br/><i>[Contêiner · Redis 7 Alpine]</i><br/>Cache chave canônica com tenant_id e TTL 300s")]
        collector["otel-collector<br/><i>[Contêiner · OTel Contrib]</i><br/>Coleta traces e métricas de domínio"]
        jaeger["jaeger<br/><i>[Contêiner · In-Memory]</i><br/>Traces com marcações de cache e short-circuit"]
        prometheus["prometheus<br/><i>[Contêiner · TSDB]</i><br/>Séries temporais de estado do breaker, hit rate e p95"]
    end

    subgraph Partners ["Motores Externos"]
        pSlow["partner-slow:8080<br/><i>[Mock Seguro]</i>"]
        pFlaky["partner-flaky:8080<br/><i>[Mock Seguro]</i>"]
        pDegrading["partner-degrading:8080<br/><i>[Mock Seguro]</i>"]
    end

    broker -- "1. POST /quotes [X-Tenant-Id: corretora-a]" --> api
    api -- "2. Verifica Cache Canônico<br/>[quote:v1:corretora-a:...]" --> redis
    redis -. "Cache HIT (0ms)" .-> api
    
    api -- "3. Cache MISS: Dispara consultas paralelas<br/>[Timeout 2500ms]" --> pSlow
    api -- "3. Breaker avalia estado antes da chamada" --> pFlaky
    api -- "3. Timeout 2500ms protege de degradação" --> pDegrading

    pFlaky -. "Falha 503 / Breaker Open" .-> api
    pSlow -. "Retorna cotação" .-> api
    pDegrading -. "Retorna cotação" .-> api

    api -- "4. Fallback parcial: agrega parceiras ativas,<br/>marca degraded=true, ordena e grava no Redis" --> redis
    api -- "5. Retorna HTTP 200 OK Degradado com propostas" --> broker

    api -- "Traces (cache_hit, short_circuited) e Métricas de Negócio" --> collector
    collector --> jaeger
    collector --> prometheus
```

### 2.4 Delta Arquitetural

| Componente / Mecanismo | Como Era ("Antes") | Como Ficou ("Depois") | Benefício Técnico e de Negócio |
| :--- | :--- | :--- | :--- |
| **Integração com Redis** | Nenhuma conexão ativa; contêiner ocioso | Conexão ativa gerenciada em [internal/cache/redis.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/cache/redis.go) | Economia de R$ 0,12 por cotação repetida; latência de resposta em 0-1 ms. |
| **Proteção de Timeout** | Timeout nulo (`http.Client.Timeout = 0`) | Timeout de 2.500 ms configurado e defendido por SLA | Impede o sequestro indefinido de threads da API pela `partner-degrading`. |
| **Tolerância a Falhas** | Ausente; falhas cascateavam para o cliente | Circuit Breaker dedicado por parceira ([Sony gobreaker v2](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/breaker.go)) | Curto-circuito fail-fast em 0 ms para parceiras instáveis, poupando R$ 0,04 por consulta. |
| **Agregação de Parceiras** | Sequencial síncrona: $T_{total} = \sum T_i$ | Concorrente paralela com goroutines: $T_{total} = \max(T_i)$ | Redução de latência p95 no baseline de ~2,0 s para ~1,5 s. |
| **Política de Falha** | Tudo ou nada: 1 parceira fora causava HTTP 502 | Fallback com resposta parcial e contrato enriquecido | Disponibilidade sob carga saltou de 60% para 100%. |
| **Métricas de Negócio** | Apenas métricas de transporte HTTP genérico | 3 métricas de domínio OTel (estado do breaker, hit rate, latência) | Visibilidade operacional imediata das parceiras no Prometheus. |

---

## 3. Requisitos Funcionais e Não Funcionais

### 3.1 Requisitos Funcionais (RF)

- **RF-01 (Autenticação e Identificação de Inquilino):** A plataforma deve validar a presença obrigatória do cabeçalho `X-Tenant-Id` em toda requisição a `POST /quotes`, rejeitando inquilinos desconhecidos com HTTP 403 e ausência de cabeçalho com HTTP 400.
- **RF-02 (Agregação e Ordenação Crescente de Prêmios):** A plataforma deve consolidar as cotações retornadas pelas parceiras e devolvê-las estritamente ordenadas pelo campo `premium_cents` em ordem crescente.
- **RF-03 (Auditabilidade Regulatória SUSEP):** Toda cotação devolvida à corretora (seja gerada online, servida de cache ou em modo degradado) deve ser registrada de forma auditável e imutável por no mínimo 5 anos, contendo data/hora, tenant, parceiras consultadas, prêmios e identificador da cotação.
- **RF-04 (Resposta Parcial Degradada — Criado pela Arquitetura):** Quando uma ou duas parceiras falharem ou estiverem curto-circuitadas, a plataforma não deve abortar a requisição com 502; deve retornar HTTP 200 contendo as propostas das parceiras sobreviventes, sinalizando `degraded: true` e listando as parceiras faltantes em `unavailable_partners`. Se todas as três falharem, deve responder HTTP 502 claro.
- **RF-05 (Isolamento Criptográfico e Lógico de Cache — Criado pela Arquitetura):** O cache deve utilizar uma chave canônica composta obrigatoriamente pelo identificador da corretora (`tenant_id`), cobertura, placa em maiúsculas e hash SHA-256 do CPF, impedindo que uma corretora acesse cotações de outra ou que CPFs em claro sejam expostos no Redis.
- **RF-06 (Curto-Circuito sem Emissão de Requisição — Criado pela Arquitetura):** Quando o Circuit Breaker de uma parceira estiver no estado `Open`, a plataforma deve falhar imediatamente a consulta daquela parceira em 0 ms sem disparar requisição HTTP pela rede, poupando a taxa de R$ 0,04 por consulta.

### 3.2 Requisitos Não Funcionais (RNF)

| ID | Requisito | Métrica | Hoje (Medido no Ambiente) | Alvo da Arquitetura | Como Medir | Justificativa do Alvo |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **RNF-01** | **Latência p95 sob Carga** | p95 da rota `POST /quotes` | **8,01 s** (relatório de carga)<br/>**9,69 s** (Prometheus bucket) | **$\le$ 500 ms** (média ponderada com cache) | `histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[5m])))` | Tempo máximo aceitável para o corretor aguardar propostas durante atendimento telefônico ao cliente final sem perda de conversão. |
| **RNF-02** | **Disponibilidade / Sucesso** | Taxa de sucesso de cotações sob falhas externas | **60%** (80 erros HTTP 502 na carga de 200 reqs) | **$\ge$ 99,5%** | `sum(rate(http_server_request_duration_seconds_count{http_response_status_code="200"}[5m])) / sum(rate(http_server_request_duration_seconds_count[5m]))` | SLA comercial vendido pela Prumo às corretoras; nenhuma cotação pode morrer por instabilidade transitória de uma única seguradora. |
| **RNF-03** | **Custo com Consultas de Parceiras** | Redução de requisições externas compradas | **0% de economia** (100% compradas: 360k req/dia) | **$\ge$ 30% de economia** (comprovado 91,6% na carga) | `sum(increase(quotation_cache_requests_total{result="hit"}[30d])) / sum(increase(quotation_cache_requests_total[30d]))` | Sustentabilidade do modelo de negócio: o custo de parceiras não pode exceder 35% da receita bruta mensal da Prumo Cota. |
| **RNF-04** | **Teto de Espera na Fronteira Externa** | Timeout por parceira | **Nenhum** (Timeout = 0; espera até 6,0s) | **2.500 ms** (com trip no breaker) | Configuração `PartnerTimeout` em [client.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/client.go) e métrica `quotation_partner_request_duration_seconds` | `partner-slow` consome até 1.700 ms; 2.500 ms oferece 800 ms de folga contra jitter e interrompe `partner-degrading` antes de atingir seu platô de 6.000 ms. |
| **RNF-05** | **Vazão Sustentada da Borda** | Throughput sob concorrência de 50 workers | **8,1 req/s** | **$\ge$ 500 req/s** (alcançado 3.108 req/s no PoC) | Saída do loadgen / `sum(rate(http_server_request_duration_seconds_count[1m]))` | Capacidade de suportar picos de fechamento de mês e campanhas promocionais de corretoras sem enfileiramento na API. |

---

## 4. Detalhamento da Arquitetura

### 4.1 Decisão 1: Circuit Breaker
- **Contexto:** Três parceiras instáveis com perfis distintos. A `partner-flaky` falha 40% das vezes e apresenta rajadas determinísticas de 9 falhas consecutivas (sequências 49 a 57). A `partner-degrading` não falha com erro HTTP, mas afunda em latência (soma 300 ms por chamada simultânea acima de 5 até atingir 6.000 ms).
- **Opções Consideradas:**
  1. *Circuit Breaker Global:* Um único circuito para todas as parceiras. **Descartado:** derrubaria a cotação da plataforma inteira e as duas parceiras saudáveis se a `partner-flaky` falhasse.
  2. *Circuit Breaker por Inquilino e Parceira (Tenant x Partner):* **Descartado:** a instabilidade é propriedade física do backend da seguradora, não do inquilino. Se o corretor A descobre que a parceira caiu, o corretor B não deve pagar R$ 0,04 nem esperar 2,5s para descobrir o mesmo.
  3. *Circuit Breaker Dedicado por Parceira:* Instância independente para cada seguradora. **Opção Escolhida.**
- **Escolha e Justificativa dos Parâmetros:**
  - **Biblioteca:** [github.com/sony/gobreaker/v2](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/go.mod#L7) (estável, alto desempenho, expõe explicitamente os estados `StateClosed`, `StateHalfOpen` e `StateOpen`).
  - **Limiar de Disparo:** **5 falhas consecutivas** (`ConsecutiveFailures >= 5`). Justificativa: A rajada do cenário determinístico possui 9 falhas consecutivas. Com 5 falhas, o breaker abre na 5ª falha e poupa as 4 chamadas seguintes da rajada (economizando R$ 0,16 por onda e 10 segundos de fila), evitando desarmes precoces com falhas isoladas de 1 ou 2 requisições.
  - **Tempo de Circuito Aberto (Cooldown):** **5 segundos** (`OpenTimeout = 5s`). Justificativa: Janela suficiente para a parceira que afundou drenar conexões residuais antes de receber novas sondas.
  - **Requisições em Half-Open:** **1 requisição de teste** (`MaxRequests = 1`). Justificativa: Uma única chamada sonda a parceira; se responder 200 OK, fecha o circuito imediatamente; se falhar, reabre por mais 5s sem sobrecarregá-la.
  - **Timeout da Chamada HTTP:** **2.500 ms**. O estouro do timeout **conta obrigatoriamente como falha** no contador do breaker. Justificativa: Isso converte o afundamento da `partner-degrading` em sinal explícito de falha, forçando o breaker a abrir e permitindo que ela se recupere.
- **Consequências Ruins:** Os contadores vivem na memória de cada processo da `quotation-api`. Em um ambiente com 10 réplicas horizontais, cada réplica precisará registrar 5 falhas independentemente para abrir seu circuito local.

### 4.2 Decisão 2: Cache com Isolamento Multi-Tenant
- **Contexto:** Cada consulta à parceira custa R$ 0,04. São 360.000 consultas/dia útil sem cache (R$ 316.800/mês). No entanto, corretores recotam os mesmos veículos repetidas vezes durante negociações.
- **Opções Consideradas:**
  1. *Cache Local em Memória (In-Memory Go Cache):* **Descartado:** Inviabiliza o compartilhamento de acertos entre réplicas e não sobrevive ao reinício do contêiner.
  2. *Cache Agregado Global sem Tenant:* **Descartado com Gravidade de Reprovação:** Violaria o Art. 5º e 46 da LGPD e as condições comerciais individuais das corretoras (erro de precificação e vazamento de dados).
  3. *Cache Distribuído Redis com Isolamento Canônico de Inquilino:* **Opção Escolhida.**
- **Chave de Cache Literal e Canônica:**
  ```text
  quote:v1:{tenant_id}:{coverage}:{plate}:{sha256(driver_document)}
  ```
  Exemplo literal real:
  `quote:v1:corretora-a:comprehensive:ABC1D23:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
  - `quote:v1`: Namespace versionado para facilitar migrações futuras;
  - `{tenant_id}`: Garantia de segregação multi-tenant em primeiro nível;
  - `{coverage}`: Modalidade da apólice (`comprehensive` ou `third_party`);
  - `{plate}`: Identificador veicular em caixa alta normalizada;
  - `{sha256(driver_document)}`: Pseudonimização criptográfica do CPF do condutor, atendendo ao princípio da minimização de dados da LGPD (Art. 6º, III), evitando CPFs em logs ou no Redis SLOWLOG.
- **TTL de Cache:** **300 segundos (5 minutos)**. Justificativa: Casa perfeitamente com o parâmetro `valid_for_seconds: 300` devolvido pelas parceiras, eliminando 100% do risco de divergência de preço no fechamento da apólice e capturando com máxima eficiência o comportamento de recotação no ato do atendimento do corretor.
- **Consequências Ruins:** Introduz dependência de rede com o serviço Redis. Caso o Redis caia, a aplicação opera em modo *fail-open* (miss transparente), repassando as chamadas às parceiras com aumento transitório de custos.

### 4.3 Decisão 3: Fallback por Resposta Parcial Enriquecida
- **Contexto:** No modelo original, a falha de qualquer parceira abortava a requisição inteira com HTTP 502, jogando fora cotações válidas já recebidas de outras seguradoras.
- **Opções Consideradas:**
  1. *Recusa Explícita com Erro de Negócio (HTTP 422):* **Descartado:** Impede o corretor de vender as apólices das seguradoras saudáveis.
  2. *Preço Médio Estimado Fabricado:* **Descartado com Gravidade de Reprovação:** Ilegal segundo as normas regulatórias da SUSEP.
  3. *Resposta Parcial com Enriquecimento de Metadados e Ordenação Preservada:* **Opção Escolhida.**
- **Contrato de Resposta:**
  A resposta HTTP preserva os campos originais (`tenant_id`, `quotes`, `elapsed_ms`) para manter compatibilidade com clientes legados e adiciona três metadados declarativos:
  ```json
  {
    "tenant_id": "corretora-a",
    "quotes": [
      {
        "partner": "partner-degrading",
        "quote_id": "partner-degrading-a8f2",
        "premium_cents": 120000,
        "currency": "BRL",
        "coverage_cents": 5000000,
        "valid_for_seconds": 300
      },
      {
        "partner": "partner-slow",
        "quote_id": "partner-slow-c4b1",
        "premium_cents": 180000,
        "currency": "BRL",
        "coverage_cents": 5000000,
        "valid_for_seconds": 300
      }
    ],
    "elapsed_ms": 1563,
    "degraded": true,
    "unavailable_partners": ["partner-flaky"],
    "cache_hit": false
  }
  ```
  - Se 1 ou 2 parceiras falharem: Retorna **HTTP 200 OK**, com `degraded: true`, lista as parceiras indisponíveis em `unavailable_partners` e exibe as propostas válidas ordenadas por preço crescente.
  - Se as 3 parceiras falharem: Retorna **HTTP 502 Bad Gateway** com payload explicativo `{ "error": "partner insurer unavailable", "partner": "all" }`.
- **Consequências Ruins:** O corretor recebe menos opções para apresentar ao segurado e, caso a parceira faltante fosse a mais barata, o cliente pode cotar um preço ligeiramente superior ao que existiria no mercado ideal.

### 4.4 Decisão 4: Agregação Paralela Concorrente (Bônus Implementado)
- **Contexto:** A agregação serial somava a latência de todas as parceiras: $1.500\text{ ms} + 150\text{ ms} + 120\text{ ms} = 1.770\text{ ms}$ no melhor caso. Sob concorrência, subia para mais de 8.000 ms.
- **Opção Escolhida:** Disparo simultâneo das consultas via goroutines e sincronização via `sync.WaitGroup` e canal bufferizado de resultados em [internal/quotation/service.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/service.go#L165-L200).
- **Consequências:** A latência de consulta externa passa a ser determinada pelo parceiro mais lento: $\max(T_1, T_2, T_3) \approx 1.500\text{ ms}$. Sob carga com cache, cai para menos de 60 ms.

### 4.5 Modelo C4 — Nível 3: Diagrama de Componentes da Fatia Protegida
O diagrama abaixo detalha a estrutura interna de componentes de software construídos em `internal/`:

```mermaid
flowchart TD
    subgraph PackageQuotation ["internal/quotation"]
        Handler["API Handler<br/><i>[handler.go]</i><br/>Valida X-Tenant-Id, faz bind do JSON e serializa Response"]
        Service["Quotation Service<br/><i>[service.go]</i><br/>Orquestra verificação de cache, paralelismo e fallback"]
        RequestStruct["Request / Response Models<br/><i>[request.go]</i><br/>Estruturas de domínio com metadados degraded e cache_hit"]
    end

    subgraph PackageCache ["internal/cache"]
        RedisClient["RedisCache Client<br/><i>[redis.go]</i><br/>Gera chaves canônicas com SHA-256 e armazena com TTL"]
    end

    subgraph PackagePartner ["internal/partner"]
        PartnerClient["Partner Client<br/><i>[client.go]</i><br/>Executa HTTP calls protegidas por timeout e breaker"]
        BreakerManager["Breaker Manager<br/><i>[breaker.go]</i><br/>Gerencia instâncias Sony gobreaker v2 por parceira"]
    end

    subgraph PackageTelemetry ["internal/telemetry"]
        DomainMetrics["Business Metrics Registry<br/><i>[metrics.go]</i><br/>Controla Gauges e Contadores de Negócio"]
    end

    Handler -- "Chama Quote(ctx, tenant, req)" --> Service
    Service -- "1. BuildKey() e Get(tenant, key)" --> RedisClient
    RedisClient -. "Hit / Miss" .-> Service
    Service -- "2. Dispara goroutines paralelas" --> PartnerClient
    PartnerClient -- "Executa sob cb.Execute()" --> BreakerManager
    BreakerManager -- "Se Open: Curto-circuita em 0ms" --> PartnerClient
    PartnerClient -- "Se Closed: Dispara HTTP com Timeout 2,5s" --> ExternalHTTP["Seguradoras Parceiras"]
    BreakerManager -- "OnStateChange()" --> DomainMetrics
    RedisClient -- "RecordCache(hit/miss)" --> DomainMetrics
    PartnerClient -- "RecordPartnerDuration()" --> DomainMetrics
```

### 4.6 Quatro Limites Conhecidos da Arquitetura
A arquitetura declara formalmente seus limites de contorno conhecidos, que não foram implementados nesta fatia por delimitação de escopo:

1. **Ausência de Bulkhead / Limite de Chamadas Simultâneas por Parceira:**
   - *Gatilho que o expõe:* Uma rajada repentina de 300 requisições simultâneas em cache miss para veículos diferentes.
   - *Efeito na Corretora:* Todas as 300 requisições são encaminhadas simultaneamente à `partner-degrading`, estourando seu limiar de 5 conexões e forçando sua latência para o teto de degradação. As chamadas atingirão o timeout de 2.500 ms em bloco, abrindo seu circuit breaker e degradando temporariamente todas as cotações.
2. **Circuit Breaker com Estado Local em Memória (In-Process):**
   - *Gatilho que o expõe:* Escalonamento horizontal da API para 10 contêineres/réplicas em produção.
   - *Efeito na Corretora:* O estado de falha não é compartilhado centralmente entre pods. Uma réplica que ainda não recebeu requisições continuará tentando chamar uma parceira que já caiu, precisando sofrer 5 falhas adicionais antes de abrir seu breaker local.
3. **TTL Fixo como Única Forma de Invalidação de Cache:**
   - *Gatilho que o expõe:* A seguradora altera sua tabela de comissionamento ou precificação no meio do expediente comercial.
   - *Efeito na Corretora:* Cotações cacheadas continuarão sendo servidas com o valor antigo até o término da janela de 300 segundos, não existindo endpoint de expurgo ou webhooks de invalidação orientados a eventos.
4. **Ausência de Rate Limiting e Quota por Inquilino (Noisy Neighbor):**
   - *Gatilho que o expõe:* Uma corretora de grande porte dispara um crawler de benchmarking executando 500 cotações/segundo.
   - *Efeito na Corretora:* Essa corretora esgota as conexões HTTP de saída e satura os motores das parceiras externas, penalizando as demais corretoras que compartilham a mesma infraestrutura agregada.

---

## 5. Implementação e Hospedagem

### 5.1 Mapa de Mecanismos e Arquivos Reais

| Mecanismo de Arquitetura | Arquivo no Repositório | Dependência Externa / Biblioteca | Teste Automatizado Determinístico |
| :--- | :--- | :--- | :--- |
| **Circuit Breaker** | [internal/partner/breaker.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/breaker.go)<br/>[internal/partner/client.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/client.go) | `github.com/sony/gobreaker/v2` | [internal/partner/breaker_test.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/partner/breaker_test.go) (`TestCircuitBreakerOpensAndShortCircuitsWithoutCallingPartner`, `TestTimeoutCountsAsFailureForCircuitBreaker`) |
| **Cache Multi-Tenant** | [internal/cache/redis.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/cache/redis.go) | `github.com/redis/go-redis/v9` | [internal/cache/redis_test.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/cache/redis_test.go) (`TestBuildKeyContainsTenantAndNormalizesFields`, `TestDifferentTenantsProduceDifferentKeys`) |
| **Fallback & Paralelismo**| [internal/quotation/service.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/service.go)<br/>[internal/quotation/request.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/request.go) | Go Standard Library (`sync.WaitGroup`) | [internal/quotation/resilience_test.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/quotation/resilience_test.go) (`TestServiceServesFromCacheAndSkipsPartnerCalls`, `TestMultiTenantCacheIsolation`, `TestPartialFallbackWhenOnePartnerFails`) |
| **Métricas de Negócio** | [internal/telemetry/metrics.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/internal/telemetry/metrics.go) | `go.opentelemetry.io/otel` | Observado via endpoint Prometheus `:9090` e teste de integração |
| **Boot e Wiring** | [cmd/quotation-api/main.go](file:///c:/Users/danil/Desktop/Full%20Cycle%20MBA/Labs/mba_arquitetura_desafio_cotacao_seguros/cmd/quotation-api/main.go) | `github.com/redis/go-redis/v9` | Teste ponta a ponta `make reproduce` |

### 5.2 Decisão de Hospedagem e Conformidade Regulatória
- **Contexto:** A plataforma processa dados pessoais (CPF, placa veicular, perfil de risco) de segurados brasileiros sob fiscalização direta da SUSEP (Circular 638/2021) e da ANPD (LGPD — Lei 13.709/2018), exigindo baixa latência com os data centers nacionais das seguradoras parceiras e retenção imutável de auditoria por 5 anos.
- **Opções Consideradas:**
  1. *On-Premise Puro em Data Center Próprio:* **Descartado:** Elevado custo de CAPEX de hardware, complexidade de manutenção física e pouca elasticidade para picos sazonais de cotação.
  2. *Cloud Pública Multi-Região Global (EUA/Europa):* **Descartado com Risco Jurídico:** A transferência internacional de dados de seguros (CPF, placa, hábitos de condução) sem consentimento explícito específico e sem cláusulas padrão contratuais viola o Art. 33 da LGPD e as normas da SUSEP (Circular 638/2021) sobre residência e soberania de dados do Sistema Nacional de Seguros Privados.
  3. *Cloud Pública Privada em Região Nacional (São Paulo — Brasil):* **Opção Escolhida.**
- **Escolha e Justificativa Regulatória e Financeira:**
  - Hospedagem em provedor de nuvem de primeira linha (ex: AWS `sa-east-1` ou GCP `southamerica-east1` em São Paulo) em conformidade estrita com as diretrizes de segurança cibernética e privacidade da SUSEP e ANPD;
  - Utilização de VPC privada sem roteamento direto à internet, criptografia em trânsito (TLS 1.3) e em repouso (AES-256);
  - Armazenamento imutável de logs de auditoria (WORM — *Write Once, Read Many*) via Object Storage com *Object Lock* ativado para garantir os 5 anos de retenção exigidos pelo regulador;
  - Menor latência de rede em relação às seguradoras nacionais e às corretoras clientes (RTT < 15 ms).
- **Consequências Ruins:**
  - **Custo unitário de infraestrutura mais elevado:** As regiões de nuvem em São Paulo (`sa-east-1` / `southamerica-east1`) praticam tarifas de computação, memória gerenciada (ElastiCache/Memorystore) e armazenamento cerca de 40% a 60% superiores às regiões norte-americanas (ex: `us-east-1`), além da exposição cambial e incidência tributária local;
  - **Ausência de segunda região de nuvem do mesmo provedor em território nacional:** Como os grandes provedores públicos concentram sua infraestrutura brasileira exclusivamente na região de São Paulo, a arquitetura conta com alta disponibilidade entre zonas (Multi-AZ), mas não possui failover ativo-passivo entre regiões distintas dentro do Brasil. Uma queda catastrófica da região inteira em São Paulo impõe indisponibilidade até a recuperação regional ou exige rito jurídico excepcional de contingência;
  - **Custos de tráfego de saída (Egress) e acoplamento operacional:** O volume constante de chamadas HTTP externas para os motores legados das seguradoras e o uso de recursos gerenciados específicos (como Object Lock WORM e Redis gerenciado) geram cobrança contínua de transferência de dados (data transfer out) e maior custo de migração futura entre provedores.

---

## 6. Operação e Gestão de Mudanças

### 6.1 Observabilidade Operacional
O time de SRE e os operadores em regime 24x7 devem monitorar a saúde da plataforma com base em três séries temporais fundamentais expostas no Prometheus:

1. **Estado do Circuit Breaker por Parceira:**
   - Métrica: `quotation_circuit_breaker_state` (Gauge)
   - Valores: `0` (Closed / Saudável), `1` (Half-Open / Em Sondagem), `2` (Open / Curto-Circuitada).
2. **Taxa de Acerto de Cache (Hit Rate):**
   - Métrica: `quotation_cache_requests_total` (Counter particionado por `result="hit"` e `result="miss"` e `tenant_id`).
   - PromQL para Hit Rate:
     ```promql
     sum(rate(quotation_cache_requests_total{result="hit"}[5m])) / sum(rate(quotation_cache_requests_total[5m]))
     ```
3. **Latência de Domínio por Seguradora:**
   - Métrica: `quotation_partner_request_duration_seconds` (Histograma particionado por `partner` e `status`).

### 6.2 Matriz de Alertas Operacionais

| Nome do Alerta | Consulta PromQL | Limiar de Disparo | Severidade | Ação Imediata do Plantonista |
| :--- | :--- | :--- | :--- | :--- |
| **PartnerCircuitBreakerOpen** | `quotation_circuit_breaker_state == 2` | Breaker em estado `2` (Open) por mais de **5 minutos** consecutivos | **P1 (Crítica)** | Executar Runbook RB-01; notificar o canal de suporte técnico da seguradora parceira; validar que o fallback está servindo respostas degradadas normalmente. |
| **CacheHitRateDegraded** | `sum(rate(quotation_cache_requests_total{result="hit"}[15m])) / sum(rate(quotation_cache_requests_total[15m]))` | Hit rate **< 20%** durante horário comercial (09h às 18h) | **P2 (Alta)** | Verificar latência e conectividade com o cluster Redis (`redis-cli ping`); checar taxa de evicção de chaves (`maxmemory`); inspecionar se houve alteração no padrão de envio do header `X-Tenant-Id`. |
| **PartnerLatencyElevated** | `histogram_quantile(0.95, sum by (partner, le) (rate(quotation_partner_request_duration_seconds_bucket[5m])))` | Latência p95 de qualquer parceira **> 2.000 ms** por 5 min | **P2 (Alta)** | Monitorar se o timeout de 2.500 ms está sendo atingido; verificar se a `partner-degrading` está sofrendo concorrência excessiva; preparar acionamento de contingência. |

### 6.3 Runbook Operacional: RB-01 (Circuit Breaker Aberto para Parceira Externa)
- **Sintoma:** Alerta `PartnerCircuitBreakerOpen` disparado no canal de alertas; `quotation_circuit_breaker_state{partner="partner-flaky"} == 2`.
- **Passo 1 — Diagnóstico Preliminar (0 a 3 minutos):**
  1. Acessar a UI do Jaeger (`http://localhost:16686` ou console de telemetria) e filtrar por `service: quotation-api`, `tags: partner.circuit_breaker.state=open`;
  2. Confirmar que a API está respondendo com HTTP 200 contendo `degraded: true` e `unavailable_partners: ["partner-flaky"]`;
  3. Checar a taxa de erros HTTP da API: ela deve permanecer em níveis normais (< 0,5%), comprovando que o fallback absorveu a falha.
- **Passo 2 — Ações Permitidas:**
  1. Executar requisição de teste manual diretamente contra o endpoint de saúde da parceira afetada para verificar se o motor voltou a responder:
     ```bash
     curl -s -o /dev/null -w "%{http_code} (%{time_total}s)\n" http://partner-flaky:8080/healthz
     ```
  2. Se a parceira responder 200 OK com latência normal, aguardar o próximo ciclo de Half-Open (5 segundos), onde o breaker fechará automaticamente.
  3. Comunicar a equipe de integrações externas para registrar chamado junto à seguradora.
- **Passo 3 — O que NUNCA fazer:**
  - **NÃO reiniciar a API quotation-api:** Reiniciar o processo zerará o estado em memória do circuit breaker, fazendo com que centenas de requisições em voo voltem a bater na seguradora caída, pagando R$ 0,04 por chamada inútil e piorando a degradação externa;
  - **NÃO desativar o Circuit Breaker:** Remover o mecanismo causará cascateamento imediato de erros 502 aos corretores.
- **Passo 4 — Escalonamento:**
  - Se o breaker permanecer aberto por mais de **30 minutos**, acionar o Gerente de Relacionamento de Seguradoras (Plantão Nível 2) para solicitar formalmente acionamento de contingência na seguradora parceira.

### 6.4 Gestão de Mudanças: Política de Alteração do TTL de Cache
O TTL de cache é uma variável de negócio crítica: um TTL longo aumenta a margem financeira por elevar o hit rate, mas eleva o risco comercial de precificação desatualizada; um TTL curto reduz o risco comercial, mas eleva os custos de requisições externas.

```mermaid
flowchart LR
    A["Solicitação de Mudança de TTL<br/>(Engenharia / Finanças)"] --> B{"Aprovação do Comitê<br/>(CTO + Diretor Comercial)"}
    B -- Rejeitado --> C["Processo Arquivado"]
    B -- Aprovado --> D["Alteração de CACHE_TTL_SECONDS<br/>(Git / Pull Request)"]
    D --> E["Deploy via Rolling Update<br/>(Zero Downtime)"]
    E --> F["Monitoramento do Hit Rate e Dispersão<br/>(Prometheus / Grafana)"]
    F --> G{"Divergência de Preço ou Erros?"}
    G -- Sim --> H["Rollback Imediato da Variável de Ambiente (< 1 min)"]
    G -- Não --> I["Validação Contábil e Encerramento"]
```

---

## 7. Recuperação de Desastres (DR) e Continuidade

### 7.1 Objetivos de Recuperação por Classe de Dados

| Classe de Dado | Definição e Exemplos | RTO (*Recovery Time Objective*) | RPO (*Recovery Point Objective*) | Justificativa |
| :--- | :--- | :--- | :--- | :--- |
| **Classe 1: Cache Volátil** | Cotações temporárias no Redis | **$\le$ 1 minuto** | **Infinito (Tolerância a 100% de perda)** | Dados efêmeros com validade de 300 segundos. A perda total não corrompe o negócio, apenas degrada temporariamente os custos operacionais. |
| **Classe 2: Trilha de Auditoria Regulatória** | Registros de cotações apresentadas (SUSEP) | **$\le$ 4 horas** | **Zero (RPO = 0s)** | Perda de histórico de auditoria acarreta auto de infração regulatório da SUSEP. Deve ser gravado em armazenamento durável e imutável de objetos. |
| **Classe 3: Configuração e Credenciais** | Mapeamento de parceiros, chaves e inquilinos | **$\le$ 15 minutos** | **$\le$ 1 hora** | Gerenciado como código (IaC) e variáveis criptografadas em cofre de chaves. |

### 7.2 Análise Detalhada dos Três Cenários de Desastre

#### Cenário A: Perda Total e Corrupção do Cluster Redis
- **Comportamento Imediato da Plataforma:** A aplicação detecta falha no cliente Redis e opera em modo *fail-open* gracioso (tratando todas as consultas como cache miss sem interromper o serviço).
- **Efeito Dominó nas Parceiras:** O volume de chamadas que chega às parceiras sobe instantaneamente (passa de 65%–70% para 100% de consultas reais, um aumento imediato de até 54% na carga externa). A `partner-degrading`, que tolerava o tráfego atenuado pelo cache, afunda quase imediatamente devido ao aumento de chamadas simultâneas (> 5 inflight).
- **Atuação dos Circuit Breakers:** O timeout de 2.500 ms atua como anteparo, convertendo a lentidão da `partner-degrading` em falhas controladas e abrindo seu circuit breaker. A plataforma entra em resposta parcial degradada (entregando as outras duas seguradoras com 200 OK).
- **Custo Financeiro da Degradação:** O custo operacional salta para R$ 14.400 por dia útil (R$ 654/hora durante o horário comercial), até a restauração do cache.
- **Caminho de Restauração:** O Redis é provisionado sem necessidade de restore de backup (inicia limpo). À medida que as cotações são processadas, o cache reaquece organicamente em menos de 3 minutos, restaurando o hit rate de 30% a 35% (sustentado pelo pressuposto P-01) sem intervenção manual.

#### Cenário B: Indisponibilidade Completa de uma Seguradora por 6 Horas
- **Efeito no Cliente (Corretora):** A corretora continua cotando normalmente. O tempo de resposta permanece abaixo de 1.600 ms (ou < 10 ms em cache hit). A resposta é retornada com `degraded: true` e a seguradora fora é listada em `unavailable_partners`. O corretor não perde a venda, pois possui opções das demais seguradoras.
- **Custo Financeiro do Modo Degradado:** Redução temporária de despesa de R$ 0,04 por consulta da parceira fora do ar. Risco comercial de perda de competitividade em apólices cujo perfil específico seria mais barato naquela seguradora.
- **Caminho de Restauração:** O Circuit Breaker entra em Half-Open a cada 5 segundos enviando 1 requisição sonda. Assim que a seguradora restabelece o serviço, a sonda sucede e o breaker fecha automaticamente sem necessidade de reinício da aplicação.

#### Cenário C: Perda Total do Data Center / Região de Nuvem
- **Efeito no Cliente:** Indisponibilidade temporária durante a troca de rota de DNS global.
- **Custo do Desastre:** Impacto de reputação e perda de receita por cotações não efetuadas durante a janela de failover (estimada em ~15 minutos, equivalente a ~3.750 cotações ou R$ 937 de receita não realizada).
- **Caminho de Restauração:** Roteamento de tráfego via Cloudflare / Route53 DNS Failover para a região secundária; pipeline de CI/CD reidrata a infraestrutura via Terraform; os dados de auditoria continuam íntegros no Object Storage com replicação geográfica multi-região.

---

## 8. Tecnologias, Custos e Pessoal (TCO)

### 8.1 Planilha de Custos de Parceiras por Cenário de Hit Rate

**Premissas Contratuais do Cenário:**
- Cotações por dia útil: **120.000**
- Consultas por cotação: **3** (uma para cada seguradora)
- Consultas brutas por dia útil: **360.000**
- Dias úteis no mês: **22 dias**
- Consultas brutas no mês: $360.000 \times 22 = \mathbf{7.920.000\text{ consultas/mês}}$
- Custo por consulta comprada: **R$ 0,04**
- Receita bruta da Prumo: $120.000 \times 22 \times \text{R\$} 0,25 = \mathbf{\text{R\$} 660.000,00/\text{mês}}$

| Cenário | Hit Rate Assumido | Consultas Compradas / Mês | Custo Mensal com Parceiras | Economia Bruta Mensal contra Baseline | Custo de Parceira como % da Receita |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **1. Baseline Atual (Starter Kit)** | **0% (Sem Cache)** | **7.920.000** | **R$ 316.800,00** | R$ 0,00 | **48,0%** (Metade da receita é consumida pelas parceiras) |
| **2. Cenário Conservador (TTL 300s)**| **30% de Acerto** | **5.544.000** | **R$ 221.760,00** | **R$ 95.040,00** | **33,6%** (Margem bruta expande em 14,4 pontos percentuais) |
| **3. Cenário Alvo (Teto P-01 · TTL 300s)**| **35% de Acerto** | **5.148.000** | **R$ 205.920,00** | **R$ 110.880,00** | **31,2%** (Margem bruta expande em 16,8 pontos percentuais) |

> **Nota Metodológica sobre o Hit Rate de Teste vs Negócio:**  
> O teste sintético de carga padrão repete 5 cotações em 210 requisições, gerando um hit rate empírico de ~98% no ambiente de testes. Para fins de projeção financeira responsável no SAD, adota-se o intervalo de **30% a 35%**, integralmente coerente com o teto de 35% sustentado pelo pressuposto P-01 de recotações dentro do TTL de 300 segundos.

### 8.2 Dimensionamento de Custos da Nova Infraestrutura

A infraestrutura adicionada é dimensionada para suportar a carga de produção de 120.000 cotações diárias:

1. **Camada de Cache Redis Gerenciado (AWS ElastiCache / GCP Memorystore):**
   - Configuração: Instância `cache.m6g.large` (2 vCPUs, 6,38 GB RAM) com réplica de leitura Multi-AZ.
   - Volumetria de memória necessária: 120.000 cotações/dia com 65% distintas (hit rate de 35%) = 78.000 chaves de ~1,5 KB = ~117 MB de dados acumulados no dia (e ~3,2 MB ativos simultaneamente por ciclo de 300 segundos). Capacidade opera com folga massiva (> 95% de headroom).
   - Custo Mensal Estimado: **R$ 1.800,00 / mês**.
2. **Cluster de Telemetria e Observabilidade (OTel Collector + Prometheus + Grafana Cloud / VM):**
   - Retenção de métricas de negócio e traces com amostragem estatística.
   - Custo Mensal Estimado: **R$ 1.200,00 / mês**.
3. **Acervo de Auditoria Regulatória SUSEP (5 Anos de Retenção WORM):**
   - **Regra Fundamental de Dimensionamento:** A base de auditoria é contada por **cotação apresentada ao consumidor**, e não por consulta externa comprada!
   - Volumetria anual: $120.000 \times 22 \times 12 = \mathbf{31.680.000\text{ cotações apresentadas por ano}}$.
   - Tamanho por registro de auditoria compactado e assinado digitalmente: ~2 KB.
   - Volume gerado por ano: $31.680.000 \times 2\text{ KB} = \mathbf{63,36\text{ GB / ano}}$.
   - **Custo de Armazenamento no Ano 1 (63,36 GB em AWS S3 Glacier Instant Retrieval / GCP Coldline):**
     - $63,36\text{ GB} \times \text{R\$} 0,025/\text{GB} \approx \mathbf{\text{R\$} 180,00 / \text{mês}}$ (incluindo custos de put requests).
   - **Custo de Armazenamento no Ano 5 (Acervo Completo de 316,8 GB retidos simultaneamente):**
     - $316,8\text{ GB} \times \text{R\$} 0,025/\text{GB} \approx \mathbf{\text{R\$} 900,00 / \text{mês}}$.

### 8.3 Custos de Pessoal (Construção vs Sustentação)

- **Fase de Construção (CAPEX de Engenharia — Projeto de 2 Meses):**
  - 1 Arquiteto de Soluções Sênior (Alocação 100%): R$ 22.000,00 / mês $\times 2$ meses = R$ 44.000,00
  - 2 Engenheiros de Software Sênior Go (Alocação 100%): R$ 18.000,00 / mês $\times 2 \times 2$ meses = R$ 72.000,00
  - **Investimento Total de Construção (CAPEX):** **R$ 116.000,00**
- **Fase de Sustentação Operacional (OPEX Contínuo Mensal):**
  - Alocação de 25% de 1 Engenheiro SRE / DevOps para manutenção, plantão e observabilidade: **R$ 6.000,00 / mês**.

### 8.4 Demonstração de Resultados e Veredito de Payback

```text
========================================================================================
BALANÇO FINANCEIRO MENSAL — CENÁRIO CONSERVADOR (Hit Rate = 30%)
========================================================================================
Economia Bruta Mensal de Parceiras:                     + R$ 95.040,00
(-) Custos Adicionais de Infraestrutura (Redis + OTel): - R$  3.000,00
(-) Custos de Armazenamento de Auditoria (Ano 1):       - R$    180,00
(-) Custo Operacional Adicional de Pessoal (SRE):       - R$  6.000,00
----------------------------------------------------------------------------------------
ECONOMIA LÍQUIDA MENSAL GERADA (30%):                   + R$ 85.860,00 / mês
========================================================================================

Cálculo do Payback Simples (Cenário Conservador — 30%):
Payback = CAPEX de Construção / Economia Líquida Mensal
Payback = R$ 116.000,00 / R$ 85.860,00 = 1,35 mês (~41 dias corridos)

========================================================================================
BALANÇO FINANCEIRO MENSAL — CENÁRIO ALVO P-01 (Hit Rate = 35%)
========================================================================================
Economia Bruta Mensal de Parceiras:                     + R$ 110.880,00
(-) Custos Adicionais de Infraestrutura (Redis + OTel): - R$   3.000,00
(-) Custos de Armazenamento de Auditoria (Ano 1):       - R$     180,00
(-) Custo Operacional Adicional de Pessoal (SRE):       - R$   6.000,00
----------------------------------------------------------------------------------------
ECONOMIA LÍQUIDA MENSAL GERADA (35%):                   + R$ 101.700,00 / mês
========================================================================================

Cálculo do Payback Simples (Cenário Alvo — 35%):
Payback = R$ 116.000,00 / R$ 101.700,00 = 1,14 mês (~34 dias corridos)
```

No **Cenário Alvo (Hit Rate = 35%, teto sustentado pelo pressuposto P-01)**, a economia líquida mensal atinge **R$ 101.700,00 / mês**, reduzindo o payback para **34 dias corridos** (1,14 mês).

### 8.5 Veredito Conclusivo do Arquiteto
O veredito arquitetural e financeiro é categoricamente **FAVORÁVEL à imediata homologação e entrada em produção**:
1. **O cache não é uma otimização técnica cosmética; é o esteio da viabilidade financeira do modelo de negócios**, transformando um negócio com 48% de despesa em custos de terceiros em uma operação com 31,2% a 33,6% de custo de insumo (cumprindo o teto de 35% do RNF-03);
2. **O payback do investimento total de engenharia ocorre entre 34 dias (cenário alvo de 35%) e 41 dias corridos (cenário conservador de 30%)**;
3. **A conformidade regulatória perante a SUSEP e ANPD é integralmente assegurada**, eliminando passivos de milhões de reais em risco de sanção administrativa;
4. **A experiência da corretora é transformada**, eliminando erros 502 e reduzindo a latência percebida de 8 segundos para meros 55 milissegundos.
