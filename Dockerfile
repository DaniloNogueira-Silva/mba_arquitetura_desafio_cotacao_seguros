# Imagem unica para todos os binarios de cmd/. Qual deles entra na imagem e decidido pelo argumento
# CMD, no docker-compose.yml — a mesma ideia dos tres perfis de parceira: um artefato, varios papeis.
# 1.25 e o piso do SDK do OpenTelemetry (go.opentelemetry.io/otel exige `go 1.25.0`), nao preferencia.
ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Baixar as dependencias antes de copiar o codigo mantem esta camada em cache: mexer no Go nao
# refaz o download do OTel a cada build.
COPY go.mod go.sum ./
RUN go mod download

# Copia o codigo inteiro (o .dockerignore corta o que nao e fonte) em vez de listar cmd/ e internal/:
# git nao versiona diretorio vazio, entao um clone novo pode nao ter todas as pastas do layout ainda.
COPY . .

ARG CMD=quotation-api
RUN CGO_ENABLED=0 go build -trimpath -o /out/app ./cmd/${CMD}

FROM alpine:3.20
# wget vem no busybox e e o que o healthcheck do compose usa.
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
