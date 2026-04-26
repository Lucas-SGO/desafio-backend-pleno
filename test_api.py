import hmac
import hashlib
import json
import time
import requests
import asyncio
import websockets

# --- config ---
BASE = "http://localhost:8080"
WEBHOOK_SECRET = "dev-webhook-secret"
JWT_SECRET = "dev-jwt-secret"
CPF = "12345678901"


def sign(payload: bytes) -> str:
    mac = hmac.new(WEBHOOK_SECRET.encode(), payload, hashlib.sha256)
    return f"sha256={mac.hexdigest()}"


def make_jwt(cpf: str) -> str:
    import base64
    header = base64.urlsafe_b64encode(b'{"alg":"HS256","typ":"JWT"}').rstrip(b"=")
    payload = base64.urlsafe_b64encode(json.dumps({
        "preferred_username": cpf,
        "exp": int(time.time()) + 3600,
    }).encode()).rstrip(b"=")
    msg = header + b"." + payload
    sig = base64.urlsafe_b64encode(
        hmac.new(JWT_SECRET.encode(), msg, hashlib.sha256).digest()
    ).rstrip(b"=")
    return (msg + b"." + sig).decode()


def test_webhook():
    print("\n=== Webhook ===")

    body = json.dumps({
        "chamado_id":      f"CH-PYTHON-{int(time.time())}",
        "tipo":            "status_change",
        "cpf":             CPF,
        "status_anterior": "aberto",
        "status_novo":     "em_execucao",
        "titulo":          "Teste Python",
        "descricao":       "Enviado via script Python",
        "timestamp":       time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }).encode()

    r = requests.post(
        f"{BASE}/webhook/events",
        data=body,
        headers={
            "Content-Type":    "application/json",
            "X-Signature-256": sign(body),
        },
    )
    print(f"[webhook válido]     {r.status_code} — {r.json()}")

    # segundo envio do mesmo body → deve retornar duplicate
    r2 = requests.post(
        f"{BASE}/webhook/events",
        data=body,
        headers={
            "Content-Type":    "application/json",
            "X-Signature-256": sign(body),
        },
    )
    print(f"[webhook duplicado]  {r2.status_code} — {r2.json()}")

    # assinatura inválida → 401
    r3 = requests.post(
        f"{BASE}/webhook/events",
        data=body,
        headers={
            "Content-Type":    "application/json",
            "X-Signature-256": "sha256=invalida",
        },
    )
    print(f"[assinatura inválida] {r3.status_code}")


def test_rest():
    print("\n=== REST API ===")
    token = make_jwt(CPF)
    headers = {"Authorization": f"Bearer {token}"}

    # primeira página
    r = requests.get(f"{BASE}/notifications", headers=headers, params={"limit": 5})
    body = r.json()
    print(f"[list página 1]  {r.status_code} — {len(body['data'])} itens, has_more={body['has_more']}")

    # próxima página via cursor
    if body["has_more"]:
        r2 = requests.get(f"{BASE}/notifications", headers=headers,
                          params={"limit": 5, "cursor": body["next_cursor"]})
        print(f"[list página 2]  {r2.status_code} — {len(r2.json()['data'])} itens")

    # contagem de não lidas
    r = requests.get(f"{BASE}/notifications/unread-count", headers=headers)
    print(f"[unread-count]   {r.status_code} — count={r.json()['count']}")

    # marcar primeira como lida
    if body["data"]:
        nid = body["data"][0]["id"]
        r = requests.patch(f"{BASE}/notifications/{nid}/read", headers=headers)
        print(f"[mark read]      {r.status_code} — {r.json()}")

        # verificar contagem diminuiu
        r = requests.get(f"{BASE}/notifications/unread-count", headers=headers)
        print(f"[unread após]    {r.status_code} — count={r.json()['count']}")

    # token inválido → 401
    r = requests.get(f"{BASE}/notifications",
                     headers={"Authorization": "Bearer token.invalido.aqui"})
    print(f"[token inválido] {r.status_code}")


async def test_websocket():
    print("\n=== WebSocket ===")
    token = make_jwt(CPF)
    uri = f"ws://localhost:8080/ws?token={token}"

    async with websockets.connect(uri) as ws:
        print("[ws] conectado — aguardando notificação por 6s...")
        print("[ws] dica: em outro terminal, execute: just seed")
        try:
            msg = await asyncio.wait_for(ws.recv(), timeout=6)
            print(f"[ws] notificação recebida: {msg}")
        except asyncio.TimeoutError:
            print("[ws] nenhuma mensagem recebida no tempo limite")


def test_healthz():
    print("\n=== Health ===")
    r = requests.get(f"{BASE}/healthz")
    print(f"[healthz] {r.status_code} — {r.json()}")


if __name__ == "__main__":
    test_healthz()
    test_webhook()
    test_rest()
    asyncio.run(test_websocket())
