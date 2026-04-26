# Desafio Técnico — Back-end Pleno (Golang)

Serviço de notificações para cidadãos acompanharem o ciclo de vida dos chamados municipais do Rio de Janeiro em tempo real.

---

## Quick Start

```bash
# Requisitos: Docker, Docker Compose, just
cp .env.example .env
just up
```

O servidor sobe em `http://localhost:8080`. Postgres e Redis são provisionados automaticamente com healthchecks — o app só inicia após ambos estarem saudáveis.

Para rodar os testes:

```bash
just test
```

Para enviar um webhook de teste (requer `curl` e `openssl`):

```bash
just seed
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

### REST API (app → serviço)

Todas as rotas exigem `Authorization: Bearer <JWT>` com `preferred_username` = CPF do cidadão.

```
GET  /notifications?page=1&limit=20
GET  /notifications/unread-count
PATCH /notifications/:id/read
```

Resposta do `GET /notifications`:
```json
{ "data": [...], "total": 42, "page": 1, "limit": 20 }
```

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
- Payload: `{ "preferred_username": "12345678901", "exp": <futuro> }`

---

## Decisões de Arquitetura

### Privacidade do CPF

O CPF **nunca** é armazenado em texto plano no banco. Antes de qualquer operação, o CPF é transformado em `HMAC-SHA256(cpf, CPF_HMAC_SECRET)`.

A escolha de HMAC sobre SHA-256 simples é proposital: sem a chave secreta, ataques de dicionário sobre o conjunto finito de CPFs brasileiros válidos (~100M) são inviáveis. O hash é determinístico, o que permite lookups sem guardar o CPF.

Toda a lógica de privacidade está em [`internal/crypto/cpf.go`](internal/crypto/cpf.go) e é chamada em exatamente dois lugares: no handler do webhook e no middleware JWT.

### Idempotência do Webhook

A tabela `event_log` tem chave primária no hash do evento (`SHA256(chamado_id|tipo|timestamp)`). O handler executa uma transação que insere primeiro no `event_log` e depois em `notifications`. Um segundo envio do mesmo evento gera violação de PK (PG code `23505`), convertida em `ErrDuplicateEvent` → HTTP 200 sem efeito colateral. A transação garante consistência entre as duas tabelas.

### Bridge Webhook → WebSocket via Redis Pub/Sub

Após salvar no banco, o handler publica em `notifications:{cpfHash}`. O `Hub` mantém uma única conexão `PSUBSCRIBE notifications:*` com o Redis, extrai o `cpfHash` do canal e despacha para os clientes conectados daquele cidadão.

Vantagens: uma conexão Redis para N clientes WebSocket; funciona com múltiplas réplicas da aplicação; a entrega WebSocket é best-effort — o REST API é a fonte autoritativa.

### JWT

O desafio não fornece chave pública ou JWKS. A implementação usa HS256 com `JWT_SECRET` configurável. Quando vazio (padrão em dev), a verificação de assinatura é ignorada — o token ainda é parseado e `preferred_username` é extraído. Para upgrade a RS256/JWKS, basta trocar `parseSigned` em [`internal/middleware/jwt.go`](internal/middleware/jwt.go).

### Auth no WebSocket

O upgrade de WebSocket é um GET; browsers não permitem headers customizados no handshake. O middleware aceita o token tanto em `Authorization: Bearer` quanto em `?token=` query param.

### Ownership nas queries

Toda query inclui `AND cpf_hash = $N`. Mesmo com um bug no middleware JWT, a query não vaza dados de outro cidadão — defesa em profundidade.

---

## Estrutura do Projeto

```
cmd/server/               entry point e wiring de componentes
internal/
  config/                 carregamento de env vars com fail-fast
  crypto/                 CPFHash — única fonte de verdade
  db/                     conexão Postgres + migrations embarcadas (embed.FS)
  domain/                 tipos puros: WebhookPayload, Notification
  middleware/             BearerJWT, WebhookSignature
  notification/           repository (SQL), service, handler HTTP
  redis/                  cliente Redis
  webhook/                handler POST /webhook/events
  ws/                     Hub, Client, handler WS /ws
internal/db/migrations/   SQL embarcado no binário via embed.FS
scripts/                  seed_webhook.sh
k6/                       load test
```

---

## Variáveis de Ambiente

| Variável | Obrigatória | Descrição |
|---|---|---|
| `APP_PORT` | não (default `8080`) | Porta HTTP |
| `DATABASE_URL` | sim | String de conexão PostgreSQL |
| `REDIS_URL` | sim | URL Redis |
| `WEBHOOK_SECRET` | sim | Chave HMAC para X-Signature-256 |
| `CPF_HMAC_SECRET` | sim | Chave HMAC para hash do CPF |
| `JWT_SECRET` | não (default `""`) | Chave HS256; vazio = skip verificação |

---

## Testes

Testes unitários rodam sem dependências externas. Testes de integração do repositório usam PostgreSQL real e são pulados quando `TEST_DATABASE_URL` não está definida.

```bash
just test   # sobe postgres + redis via docker compose e roda go test ./...
```

Cada teste de integração cria um schema isolado (`test_<timestamp>`) e remove ao final via `DROP SCHEMA CASCADE`.

### Load Test (k6)

```bash
just k6   # requer k6 instalado
```

20 VUs por 30 segundos — thresholds: p95 < 500ms, error rate < 1%.

---

## O que faria com mais tempo

- **RS256 / JWKS**: buscar a chave pública do IDP em `/jwks.json` na inicialização com suporte a rotação de chaves.
- **Dead Letter Queue**: eventos com falha de persistência iriam para uma fila Redis e seriam reprocessados por um worker separado.
- **Circuit Breaker**: proteger chamadas ao banco e ao Redis com `sony/gobreaker` para degradação graciosa.
- **OpenTelemetry**: traces do webhook até o WebSocket, exportados para Jaeger.
- **Paginação por cursor**: offset degrada em tabelas grandes; cursor-based (por `created_at + id`) é mais adequado para produção.
- **Kubernetes manifests**: Deployment, Service, HPA baseado em conexões WebSocket ativas.

---

Dúvidas: **selecao.pcrj@gmail.com**
