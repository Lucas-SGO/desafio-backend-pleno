# Desafio Técnico — Back-end Pleno (Golang)

Serviço de notificações para cidadãos acompanharem o ciclo de vida dos chamados municipais do Rio de Janeiro em tempo real.

---

## Quick Start

**Requisitos:** Docker, Docker Compose, [just](https://github.com/casey/just)

```bash
cp .env.example .env
just up
```

O servidor sobe em `http://localhost:8080`. Postgres, Redis e Jaeger são provisionados automaticamente com healthchecks — o app só inicia após banco e cache estarem saudáveis.

```bash
just seed    # envia um webhook de teste com HMAC válido
just test    # roda toda a suite de testes
just k6      # load test (requer k6 instalado)
```

---

## Endpoints

### Webhook (sistema municipal → serviço)

```
POST /webhook/events
Content-Type: application/json
X-Signature-256: sha256=<HMAC-SHA256 do body com WEBHOOK_SECRET>
```

| Status | Significado |
|---|---|
| `201` | Evento processado e notificação criada |
| `200 {"status":"duplicate"}` | Evento já processado (idempotência) |
| `401` | Assinatura ausente ou inválida |
| `400` | Campos obrigatórios ausentes |

Payload esperado:
```json
{
  "chamado_id":      "CH-2024-001234",
  "tipo":            "status_change",
  "cpf":             "12345678901",
  "status_anterior": "em_analise",
  "status_novo":     "em_execucao",
  "titulo":          "Buraco na Rua",
  "descricao":       "Equipe designada para reparo",
  "timestamp":       "2024-11-15T14:30:00Z"
}
```

### REST API (app → serviço)

Todas as rotas exigem `Authorization: Bearer <JWT>` com `preferred_username` = CPF do cidadão.

```
GET  /notifications?limit=20
GET  /notifications?limit=20&cursor=<token>
GET  /notifications/unread-count
PATCH /notifications/:id/read
GET  /healthz
```

Resposta do `GET /notifications`:
```json
{
  "data": [...],
  "next_cursor": "eyJjcmVhdGVkX2F0IjoiMjAyNC0xMS0xNVQxNDozMDowMFoiLCJpZCI6Ii4uLiJ9",
  "has_more": true,
  "limit": 20
}
```

A paginação é baseada em cursor keyset `(created_at, id)` — sem `COUNT(*)`, custo constante independente do tamanho da tabela. Passe `next_cursor` da resposta anterior para buscar a próxima página.

### WebSocket

```
WS /ws?token=<JWT>
```

O cliente recebe a notificação em JSON assim que o webhook é processado, sem polling.

---

## Gerar um JWT para testes

Usando [jwt.io](https://jwt.io) ou qualquer ferramenta HS256:

- Algorithm: `HS256`
- Secret: valor de `JWT_SECRET` (em dev, qualquer valor; assinatura é ignorada quando vazio)
- Payload: `{ "preferred_username": "12345678901", "exp": <timestamp futuro> }`

---

## Variáveis de Ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `APP_PORT` | não (default `8080`) | Porta HTTP |
| `DATABASE_URL` | sim | String de conexão PostgreSQL |
| `REDIS_URL` | sim | URL Redis |
| `WEBHOOK_SECRET` | sim | Chave HMAC para validar X-Signature-256 |
| `CPF_HMAC_SECRET` | sim | Chave HMAC para hash irreversível do CPF |
| `JWT_SECRET` | não (default `""`) | Chave HS256; vazio = skip verificação (dev) |
| `OTEL_ENDPOINT` | não | Endpoint OTLP gRPC, ex: `jaeger:4317`; vazio = tracing desativado |

---

## Arquitetura

### Privacidade do CPF

O CPF **nunca** é armazenado em texto plano. Antes de qualquer operação é transformado em `HMAC-SHA256(cpf, CPF_HMAC_SECRET)`. HMAC sobre SHA-256 simples porque o espaço de CPFs válidos é finito (~100M) — sem a chave secreta, ataques de dicionário são inviáveis. O hash é determinístico, permitindo lookups sem guardar o CPF. Toda a lógica está em [`internal/crypto/cpf.go`](internal/crypto/cpf.go) e é chamada em exatamente dois lugares: handler webhook e middleware JWT.

### Idempotência do Webhook

A tabela `event_log` tem chave primária no hash do evento (`SHA256(chamado_id|tipo|timestamp)`). O handler executa uma transação que insere primeiro no `event_log` e depois em `notifications`. Um segundo envio gera violação de PK (PG `23505`), convertida em `ErrDuplicateEvent` → HTTP 200 sem efeito colateral. A transação garante atomicidade entre as duas tabelas.

### Bridge Webhook → WebSocket via Redis Pub/Sub

Após salvar no banco, o handler publica em `notifications:{cpfHash}`. O Hub mantém uma única conexão `PSUBSCRIBE notifications:*` com o Redis, extrai o `cpfHash` do canal e despacha para os clientes WebSocket daquele cidadão. Funciona com múltiplas réplicas da aplicação sem nenhuma coordenação adicional.

### Dead Letter Queue

Falhas transientes de persistência não perdem o evento. O `Service` enfileira o payload em `dlq:webhook:events` (Redis LIST). Um worker goroutine consome com `BRPOP`, retenta com backoff exponencial (`2^retries` segundos) e, após 3 falhas, move para `dlq:webhook:dead` para inspeção manual. Duplicatas detectadas durante o reprocessamento são descartadas silenciosamente.

### Circuit Breaker

`breakeredRepository` decora o repositório Postgres com `sony/gobreaker`. Após 5 falhas consecutivas o breaker abre por 30 segundos, rejeitando chamadas imediatamente em vez de acumular goroutines aguardando timeout de conexão. O padrão decorator não altera a interface `Repository` — zero mudança nos callers.

### Paginação por Cursor

`GET /notifications` usa keyset pagination sobre `(created_at DESC, id DESC)`. O cursor é `base64url(timestamp|uuid)` do último item retornado. A query usa `WHERE (created_at, id) < ($ts, $id)` aproveitando o índice composto — custo constante independente do offset, sem `COUNT(*)`.

### OpenTelemetry + Jaeger

Traces distribuídos do handler HTTP até o SQL e o `redis.Publish`. Quando `OTEL_ENDPOINT` está vazio (dev sem Jaeger), o tracer é no-op sem nenhum custo. Com `just up`, a UI do Jaeger fica disponível em `http://localhost:16686`.

### JWT

O desafio não fornece JWKS. A implementação usa HS256 com `JWT_SECRET` configurável. Quando vazio, a assinatura é ignorada e `preferred_username` é extraído normalmente — adequado para dev. Para upgrade a RS256/JWKS, basta substituir o parser em [`internal/middleware/jwt.go`](internal/middleware/jwt.go).

### Ownership nas queries

Toda query ao banco inclui `AND cpf_hash = $N`. Mesmo com um bug no middleware JWT, a query não vaza dados de outro cidadão — defesa em profundidade.

---

## Estrutura do Projeto

```
cmd/server/               entry point e wiring de todos os componentes
internal/
  config/                 carregamento de env vars com fail-fast
  crypto/                 CPFHash — única fonte de verdade do hash do CPF
  db/                     conexão Postgres + migrations embarcadas (embed.FS)
  dlq/                    Dead Letter Queue — worker BRPOP + backoff exponencial
  domain/                 tipos puros: WebhookPayload, Notification
  middleware/             BearerJWT, WebhookSignature
  notification/           repository (SQL), service, handler HTTP
  redis/                  cliente Redis
  telemetry/              inicialização do TracerProvider OTLP
  webhook/                handler POST /webhook/events
  ws/                     Hub, Client, handler WS /ws
internal/db/migrations/   SQL embarcado no binário via embed.FS
k8s/                      manifests Kubernetes (Namespace, Deployment, Service, HPA)
scripts/                  seed_webhook.ps1
k6/                       load test
```

---

## Testes

Testes unitários rodam sem dependências externas. Testes de integração do repositório usam PostgreSQL real e são pulados quando `TEST_DATABASE_URL` não está definida.

```bash
just test   # sobe postgres + redis via docker compose e roda go test ./...
```

Cada teste de integração cria um schema isolado (`test_<timestamp>`) e faz `DROP SCHEMA CASCADE` ao final.

### Load Test (k6)

```bash
just k6   # requer k6 instalado
```

20 VUs por 30 segundos — thresholds: p95 < 500ms, error rate < 1%.

### Testes em Python

Além da suite Go, há um script Python que exercita todos os fluxos contra o servidor rodando:

```bash
pip install requests websockets
python test_api.py
```

Cobre: `GET /healthz`, webhook válido/duplicado/assinatura inválida, listagem com cursor, unread-count, mark-read, token inválido e conexão WebSocket.

---

## Kubernetes

```bash
kubectl apply -f k8s/
```

Edite `k8s/secret.yaml` com os valores reais antes de aplicar. O Deployment usa liveness/readiness probes em `/healthz`, começa com 2 réplicas e o HPA escala até 10 baseado em CPU (70% de utilização média).

---

## Verificação end-to-end

```bash
just up          # sobe postgres, redis, jaeger e a aplicação

just seed        # envia webhook com HMAC válido → deve retornar 201

# Gerar JWT em jwt.io (HS256, preferred_username=12345678901)
curl -H "Authorization: Bearer <token>" localhost:8080/notifications
curl -H "Authorization: Bearer <token>" localhost:8080/notifications/unread-count

# WebSocket em tempo real
wscat -c "ws://localhost:8080/ws?token=<token>"
# em outro terminal: just seed → a notificação chega no wscat

# Traces no Jaeger
open http://localhost:16686   # buscar serviço "notificacoes"
```
