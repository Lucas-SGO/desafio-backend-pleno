#!/usr/bin/env bash
set -e

HOST="${HOST:-http://localhost:8080}"
SECRET="${WEBHOOK_SECRET:-dev-webhook-secret}"

PAYLOAD='{"chamado_id":"CH-2024-001234","tipo":"status_change","cpf":"12345678901","status_anterior":"em_analise","status_novo":"em_execucao","titulo":"Buraco na Rua — Atualização","descricao":"Equipe designada para reparo na Rua das Laranjeiras, 100","timestamp":"2024-11-15T14:30:00Z"}'

SIG="sha256=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')"

echo "→ Sending webhook to $HOST/webhook/events"
curl -s -w "\nHTTP %{http_code}\n" \
  -X POST "$HOST/webhook/events" \
  -H "Content-Type: application/json" \
  -H "X-Signature-256: $SIG" \
  -d "$PAYLOAD"
