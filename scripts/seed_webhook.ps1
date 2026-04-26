$host_url = if ($env:HOST) { $env:HOST } else { "http://localhost:8080" }
$secret   = if ($env:WEBHOOK_SECRET) { $env:WEBHOOK_SECRET } else { "dev-webhook-secret" }

$payload = '{"chamado_id":"CH-2024-001234","tipo":"status_change","cpf":"12345678901","status_anterior":"em_analise","status_novo":"em_execucao","titulo":"Buraco na Rua — Atualizacao","descricao":"Equipe designada para reparo na Rua das Laranjeiras, 100","timestamp":"2024-11-15T14:30:00Z"}'

$hmac = [System.Security.Cryptography.HMACSHA256]::new([System.Text.Encoding]::UTF8.GetBytes($secret))
$hash = $hmac.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($payload))
$sig  = "sha256=" + ([System.BitConverter]::ToString($hash) -replace "-","").ToLower()

Write-Host "-> Sending webhook to $host_url/webhook/events"
Invoke-RestMethod -Uri "$host_url/webhook/events" `
    -Method POST `
    -ContentType "application/json" `
    -Headers @{"X-Signature-256" = $sig} `
    -Body $payload | ConvertTo-Json
