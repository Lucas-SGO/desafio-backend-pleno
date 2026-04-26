import http from "k6/http";
import { check, sleep } from "k6";
import { crypto } from "k6/experimental/webcrypto";

export const options = {
  vus: 20,
  duration: "30s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
  },
};

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const WEBHOOK_SECRET = __ENV.WEBHOOK_SECRET || "dev-webhook-secret";

async function sign(body) {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(WEBHOOK_SECRET),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const sig = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(body));
  const hex = Array.from(new Uint8Array(sig))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return `sha256=${hex}`;
}

export default async function () {
  const id = Math.floor(Math.random() * 1000000);
  const ts = new Date().toISOString().replace(/\.\d+Z$/, "Z");

  const payload = JSON.stringify({
    chamado_id: `CH-${id}`,
    tipo: "status_change",
    cpf: "12345678901",
    status_anterior: "aberto",
    status_novo: "em_analise",
    titulo: "Buraco na rua",
    descricao: "Rua das Laranjeiras, 100",
    timestamp: ts,
  });

  const sig = await sign(payload);

  const res = http.post(`${BASE_URL}/webhook/events`, payload, {
    headers: {
      "Content-Type": "application/json",
      "X-Signature-256": sig,
    },
  });

  check(res, {
    "webhook 201 or 200": (r) => r.status === 201 || r.status === 200,
  });

  sleep(0.1);
}
