# Guia das oito seções do SAD

O que se espera de cada seção do `docs/sad.md` da entrega 1. Este documento é anexo do enunciado
(`README.md` do starter, `docs/enunciado.md` depois que você o substituir), e não substitui nada
que está lá: as **três regras** que valem para o documento inteiro (rastreabilidade, números em vez
de adjetivos, e diagrama em C4 com Mermaid), os **critérios de aceite** contáveis e o **fluxo do
avaliador** continuam no enunciado, e é contra eles que a entrega é conferida.

Cada seção abaixo traz o que ela precisa conter e, quando existe, o contra-exemplo que costuma
aparecer no lugar do conteúdo.

## 1. Introdução: propósito, escopo, restrições, pressupostos

Diga para quem o documento serve e qual decisão ele sustenta. Delimite o escopo pelos dois lados: o
SAD cobre a plataforma de cotação e a sua **fronteira** com as seguradoras, e não redesenha o motor
de precificação delas nem o CRM da corretora. Separe o que é **restrição** (imposta: Go, ambiente
local sem custo, três parceiras, cobrança por consulta, retenção de cinco anos, papel de operadora)
do que é **decisão** (sua: TTL, limiares do breaker, onde a plataforma roda); confundir os dois faz
o documento perder autoridade, porque ninguém defende uma escolha que apresenta como imposição. E
declare cada pressuposto com valor, origem e **consequência se ele for falso**: "assumo que 30% das
cotações se repetem em até uma hora; se for 5%, o cache não se paga e a seção 8 muda de veredito".

- **Não conta:** pressuposto sem consequência declarada. É enfeite, não pressuposto.

## 2. Visão geral da arquitetura

Uma página com o **antes** e o **depois**, nos diagramas C4 de nível 1 e 2. O "antes" não é
imaginação: os contêineres estão no `docker-compose.yml` e os nomes têm que bater (`quotation-api`,
`partner-slow`, `partner-flaky`, `partner-degrading`, `redis`, `otel-collector`, `jaeger`,
`prometheus`). O nível 1 mostra os atores externos, a corretora e as três seguradoras, e deixa
visível o que o Cenário do enunciado afirma: a Prumo intermedeia, não precifica. Feche com o
**delta** em lista, o que você acrescenta, o que muda de lugar e o que sai, e um parágrafo curto de
"a quem esta arquitetura serve".

- **Reprova:** contêiner no diagrama que não existe nem no compose nem na sua entrega. É o caso mais
  comum de violação da regra 1.

## 3. Requisitos funcionais e não funcionais

Os funcionais saem do contrato que já existe, não da sua cabeça: `POST /quotes` exige `X-Tenant-Id`,
a resposta agrega as três parceiras ordenadas por prêmio (`internal/quotation/service.go`), toda
cotação apresentada fica auditável por cinco anos, e resposta de cache ou de fallback continua
rastreável até a consulta que a originou. Escreva cada um como frase testável, com identificador
(RF-01 em diante), e acrescente os que a **sua** arquitetura cria. Os não funcionais vão em tabela,
e o "hoje" é medido por você com `make reproduce`, não estimado:

| ID | Requisito | Métrica | Hoje | Alvo | Como medir | Por que este número |
|---|---|---|---|---|---|---|
| RNF-01 | Latência da cotação | p95 do `POST /quotes` | 8,01 s sob carga (medição do roteiro, feita em outra máquina; use a sua) | *sua decisão* | `http_server_request_duration_seconds` | *sua justificativa* |

A coluna que separa o arquiteto do preenchedor de template é a última. Um alvo de p95 vem do tempo
que o corretor aguenta esperar com o cliente na frente dele, do SLA que a Prumo vende, ou da soma
das parceiras no melhor caso. "Porque é um bom número" não é origem.

- **Não conta:** qualquer não funcional sem número ou sem método de medição.

## 4. Detalhamento da arquitetura

O coração do documento. Cada decisão no formato **contexto → opções consideradas → escolha →
consequências, inclusive as ruins**, e no mínimo três: circuit breaker, cache e fallback. Todo
parâmetro numérico precisa da sua defesa: por que o breaker abre com N falhas em uma janela de M
segundos e não com outro par, quantas requisições o meio aberto deixa passar, qual o TTL do cache
diante do teto de 24 horas e por que parar antes dele. Escreva a **chave do cache literalmente**: se
`tenant_id` não estiver nela, você projetou um incidente de dados pessoais, não uma otimização. E
diga o que o fallback devolve, como o cliente sabe que aquilo é fallback, e como a resposta segue
auditável. O C4 de nível 3 mora aqui, cobrindo a fatia protegida: `internal/partner/client.go` e
`internal/quotation/service.go`.

As consequências ruins incluem o que você **não** resolveu, e é por isso que o aceite pede quatro
limites conhecidos declarados, com o gatilho que expõe cada um e o efeito na corretora. Nenhum deles
é para implementar — a fronteira entre o que a arquitetura cobre e o que ela deixa de fora faz parte
da arquitetura, e limite calado é o que vira incidente às 3h da manhã com o plantonista descobrindo
sozinho. Declarar que o seu breaker vive na memória do processo, e portanto que cada réplica aprende
sozinha que a parceira caiu, vale mais do que uma seção a mais sobre o que você implementou.

- **Não conta:** "usaremos circuit breaker" sem os três estados, sem limiar e sem janela. É
  exatamente a frase que este desafio existe para tornar impossível.

## 5. Implementação

Duas perguntas. **Como se constrói:** o mapa de cada mecanismo para o arquivo real do repositório
onde ele nasce, qual biblioteca e por que ela (a que expõe os três estados explicitamente vale mais
que a mais estrelada), e como o comportamento é testado sem depender de sorte, no padrão de
`cmd/partner-mock/feasibility_test.go`. **Onde roda:** on-premise, cloud ou híbrido, decidido pelo
compliance do domínio, com pelo menos uma alternativa descartada e o motivo regulatório e financeiro
dela. Some o custo, que reaparece na seção 8.

- **Não conta:** "cloud é o padrão de mercado". Isso não é argumento, é ausência de argumento.

## 6. Operação e gestão de mudanças

O SAD é lido em produção, sob pressão. A observabilidade da entrega 2 é o instrumento desta seção:
diga **o que se olha** (estado do breaker, hit rate do cache, latência por parceira), **qual limiar
dispara alerta** e **qual ação o alerta exige**. Escreva pelo menos um **runbook** completo de um
cenário real, como "o breaker da `partner-degrading` está aberto há dez minutos", com o que o
plantonista faz, o que ele **não** faz e quando escala. E trate a gestão de mudanças pelo caso que
importa aqui: o TTL do cache é parâmetro de **negócio**, porque mexe em risco de preço vencido e em
custo. Quem aprova mudá-lo, como ele chega em produção, como se reverte e como se mede se melhorou.

- **Não conta:** "monitoraremos com Prometheus e Grafana". Isso é uma lista de compras. Alerta sem
  limiar e sem ação também não é alerta, é ruído.

## 7. Recuperação de desastres

RTO e RPO com número, e **por classe de dado**, porque elas não valem o mesmo: perder o cache custa
dinheiro e latência, perder o registro de auditoria é infração regulatória. Cubra no mínimo três
cenários: (a) o Redis inteiro se perde; (b) uma parceira fica fora por seis horas; (c) perda do site
ou da região. Para cada um: o que a corretora vê, o que **degrada** e o que **para** (não é a mesma
coisa), quanto custa em reais o modo degradado, e o caminho de volta. O cenário (a) tem endereço na
seção 8: cache vazio é 100% das consultas compradas de novo. Mas ele não para na conta, e é aí que o
cenário fica interessante: as mesmas consultas que voltam a ser compradas voltam a ser **feitas ao
mesmo tempo**, e o volume que chega às parceiras mais que dobra a partir de 50% de acerto. A
`partner-degrading` afunda acima de cinco chamadas simultâneas, até o teto de 6000 ms. Perder o Redis,
portanto, não é só ficar mais caro: é empurrar uma parceira para o colapso. Diga qual dos dois efeitos
chega primeiro e o que a corretora vê enquanto isso.

- **Não conta:** um RPO único para todas as classes de dado. É sinal de que a seção não foi pensada.

## 8. Tecnologias, custos e pessoal (TCO)

A seção que fecha o circuito: a linha de código da entrega 2 tem que aparecer aqui como dinheiro.
Os números de partida estão no Cenário do enunciado. Entregue quatro coisas. Primeira, **a conta de
parceiro por cenário**: uma linha para hoje (hit rate zero) e uma para cada hit rate que o **seu**
TTL sustenta, com hit rate assumido, consultas compradas por mês, custo mensal, economia contra hoje
e percentual da receita. Segunda, **o custo da infraestrutura que você acrescenta**: Redis, retenção
de traces e métricas, e o armazenamento de auditoria, que **cresce todo mês** e precisa de cinco
anos, então mostre pelo menos o ano 1 e o ano 5. A unidade da auditoria é **cotação apresentada**, não
consulta comprada, e a distância entre as duas é o ponto: o cache derruba a consulta e não derruba o
registro, porque cotação servida de cache continua sendo cotação apresentada a um consumidor. Quem
dimensionar o acervo pelas consultas que sobraram erra para baixo pelo inverso do hit rate, o que com
60% de acerto é 2,5 vezes, e erra mais em reais justamente no ano 5, quando os cinco anos estão todos
guardados ao mesmo tempo. Terceira, **pessoal**: quantas pessoas, quais papéis, para construir e
depois para operar, com custo mensal. Quarta, **o veredito**: o cache se
paga, em quanto tempo? Se a resposta for "não", diga; um SAD que conclui contra a solução preferida,
com a conta na mão, vale mais que um que conclui a favor sem ela.

**É aqui que o documento é testado por coerência.** O hit rate desta seção tem que ser compatível
com o TTL da seção 4 e com o pressuposto de recotação da seção 1. TTL de quinze minutos sustentando
70% de acerto exige explicação; sem ela, o número é chute, e um chute na planilha contamina a
decisão que ele sustenta.

- **Não conta:** preço de instância copiado de calculadora sem dizer quantas instâncias e por quê.

## O que este SAD não é

Não é um template preenchido, e a diferença é verificável de fora: **cada seção referencia outra**.
O requisito da seção 3 aparece como mecanismo na 4, como arquivo na 5, como alerta na 6, como
cenário de desastre na 7 e como reais na 8. Um documento em que as oito seções poderiam ser lidas em
qualquer ordem, sem que nada quebrasse, é oito documentos curtos, não um SAD.
