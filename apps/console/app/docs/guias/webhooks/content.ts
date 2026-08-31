import type { GuideContent } from "../../docs-ui";

const CURL_SUB = `curl -s -X POST $API/v1/webhook-subscriptions \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "url": "https://api.acme.example/hooks/ledgercore",
    "event_types": ["ledger.transaction.posted", "ledger.transaction.reversed"]
  }'`;

const RESP_SUB = `{
  "id": "…",
  "url": "https://api.acme.example/hooks/ledgercore",
  "event_types": ["ledger.transaction.posted", "ledger.transaction.reversed"],
  "active": true,
  "created_at": "2026-07-24T12:00:00Z",
  "secret": "lcwh_4f3e2d1c0b9a"
}`;

const HEADER_EXAMPLE = `X-LedgerCore-Signature: t=1753358400,v1=5257a869e7ecebeda32affa62cdca3fa51cad7e77a0e56ff536d0ce8e108d8bd

# Durante una rotación (24 h de gracia): DOS entradas v1 — nueva primero, anterior después
X-LedgerCore-Signature: t=1753358400,v1=<nuevo-secreto-hmac>,v1=<secreto-anterior-hmac>`;

const TS_VERIFY = `// Con el SDK (recomendado): tiempo constante + ventana anti-replay de 5 min
import { LedgerCore } from "@ledgercore/sdk";
const lc = new LedgerCore({ apiKey: "lk_..." });

// ¡SIEMPRE el cuerpo crudo, nunca JSON re-serializado!
const ok = await lc.webhooks.verifySignature(
  rawBody,
  req.headers["x-ledgercore-signature"],
  "lcwh_...",
);
if (!ok) return res.status(400).end();`;

const TS_MANUAL = `// A mano, si no usas el SDK
import { createHmac, timingSafeEqual } from "node:crypto";

function verify(secret: string, header: string, rawBody: Buffer, tolerance = 300): boolean {
  const parts = header.split(",").map((p) => p.split("="));
  const t = Number(parts.find(([k]) => k === "t")?.[1]);
  if (!Number.isFinite(t) || Math.abs(Date.now() / 1000 - t) > tolerance) return false;

  const expected = createHmac("sha256", secret)
    .update(\`\${t}.\`).update(rawBody).digest();

  // acepta si CUALQUIER v1 coincide (rotación: viajan dos firmas)
  return parts
    .filter(([k]) => k === "v1")
    .some(([, v]) => {
      const sig = Buffer.from(v, "hex");
      return sig.length === expected.length && timingSafeEqual(sig, expected);
    });
}`;

const PHP_VERIFY = `// Con el SDK (hash_equals = comparación en tiempo constante)
use LedgerCore\\Webhook;

$rawBody = file_get_contents('php://input'); // cuerpo crudo, SIEMPRE
$ok = Webhook::verifySignature(
    $rawBody,
    $_SERVER['HTTP_X_LEDGERCORE_SIGNATURE'] ?? null,
    'lcwh_...',
);
if (!$ok) {
    http_response_code(400);
    exit;
}`;

const CURL_ROTATE = `curl -s -X POST $API/v1/webhook-subscriptions/$SUB_ID/rotate-secret \\
  -H "Authorization: Bearer $TOKEN"`;

const RESP_ROTATE = `{
  "id": "…",
  "secret": "lcwh_NUEVO…",
  "previous_secret_expires_at": "2026-07-25T12:00:00Z"
}`;

export const GUIDE: GuideContent = {
  es: {
    badge: "Guía · Webhooks",
    title: "Webhooks: entregas firmadas, verificadas y rotables",
    subtitle:
      "Cada entrega llega firmada con HMAC-SHA256 en la cabecera X-LedgerCore-Signature (t=…,v1=…). Verifica en tiempo constante, rechaza timestamps fuera de la ventana de 5 minutos y rota el secreto sin perder eventos gracias a las 24 h de gracia.",
    backLabel: "Docs",
    copy: "Copiar",
    copied: "Copiado",
    sections: [
      {
        title: "Crea la suscripción",
        body: "Registra tu endpoint y los tipos de evento (ledger.transaction.posted, ledger.transaction.reversed, ledger.hold.*, recon.discrepancy.detected…). El secreto de firma lcwh_… se devuelve UNA sola vez: guárdalo de inmediato. En el entorno live la URL debe ser HTTPS. Las entregas fallidas se reintentan con backoff exponencial (5 intentos, luego dead; un retry manual las re-encola).",
        blocks: [
          { kind: "code", label: "POST /v1/webhook-subscriptions", code: CURL_SUB },
          { kind: "resp", label: "Respuesta (201) — el secreto se muestra una sola vez", code: RESP_SUB },
        ],
      },
      {
        title: "La firma: t= y v1=",
        body: "El MAC se calcula sobre \"<t>.\" + cuerpo crudo con HMAC-SHA256 y tu secreto. t es el timestamp unix de la firma. Durante una rotación la cabecera trae DOS entradas v1 (secreto nuevo primero, anterior después): acepta si CUALQUIERA coincide.",
        blocks: [{ kind: "resp", label: "Cabecera de cada entrega", code: HEADER_EXAMPLE }],
      },
      {
        title: "Verifica en TypeScript",
        body: "Tres reglas de oro: usa el cuerpo crudo (no JSON re-serializado), compara en tiempo constante y rechaza timestamps fuera de tu ventana de tolerancia (recomendado: 5 minutos) para frenar replays.",
        blocks: [
          { kind: "code", label: "TypeScript · SDK", code: TS_VERIFY },
          { kind: "code", label: "TypeScript · verificación manual", code: TS_MANUAL },
        ],
      },
      {
        title: "Verifica en PHP",
        body: "Misma semántica con hash_equals. El cuerpo crudo en PHP es file_get_contents('php://input') — jamás re-encodees $_POST.",
        blocks: [{ kind: "code", label: "PHP · SDK", code: PHP_VERIFY }],
      },
      {
        title: "Rota el secreto sin perder eventos",
        body: "POST …/rotate-secret devuelve el secreto nuevo (una sola vez) y previous_secret_expires_at. Desde ese momento y durante 24 h cada entrega se firma con AMBOS secretos, así que puedes cambiar el secreto de tu verificador en cualquier punto de la ventana sin rechazar ni perder ningún evento.",
        blocks: [
          { kind: "code", label: "POST /v1/webhook-subscriptions/{id}/rotate-secret", code: CURL_ROTATE },
          { kind: "resp", label: "Respuesta — el nuevo secreto se muestra una sola vez", code: RESP_ROTATE },
        ],
      },
      {
        title: "Después de la ventana de gracia",
        body: "Pasadas las 24 h el secreto anterior deja de firmar y se purga de la base de datos. Un receptor anclado al secreto viejo dejará de verificar: rota siempre dentro de la ventana. Rotar de nuevo durante una gracia activa reemplaza el secreto anterior por el vigente y reinicia las 24 h; el penúltimo queda invalidado de inmediato.",
      },
    ],
  },
  en: {
    badge: "Guide · Webhooks",
    title: "Webhooks: signed, verified, rotatable deliveries",
    subtitle:
      "Every delivery arrives HMAC-SHA256 signed in the X-LedgerCore-Signature header (t=…,v1=…). Verify in constant time, reject timestamps outside the 5-minute window, and rotate the secret without losing events thanks to the 24 h grace period.",
    backLabel: "Docs",
    copy: "Copy",
    copied: "Copied",
    sections: [
      {
        title: "Create the subscription",
        body: "Register your endpoint and the event types (ledger.transaction.posted, ledger.transaction.reversed, ledger.hold.*, recon.discrepancy.detected…). The lcwh_… signing secret is returned exactly ONCE: store it immediately. In the live environment the URL must be HTTPS. Failed deliveries are retried with exponential backoff (5 attempts, then dead; a manual retry re-enqueues them).",
        blocks: [
          { kind: "code", label: "POST /v1/webhook-subscriptions", code: CURL_SUB },
          { kind: "resp", label: "Response (201) — the secret is shown exactly once", code: RESP_SUB },
        ],
      },
      {
        title: "The signature: t= and v1=",
        body: "The MAC is computed over \"<t>.\" + raw body with HMAC-SHA256 and your secret. t is the unix timestamp of the signature. During a rotation the header carries TWO v1 entries (new secret first, previous one second): accept if ANY matches.",
        blocks: [{ kind: "resp", label: "Header on every delivery", code: HEADER_EXAMPLE }],
      },
      {
        title: "Verify in TypeScript",
        body: "Three golden rules: use the raw body (not re-serialized JSON), compare in constant time, and reject timestamps outside your tolerance window (recommended: 5 minutes) to stop replays.",
        blocks: [
          { kind: "code", label: "TypeScript · SDK", code: TS_VERIFY },
          { kind: "code", label: "TypeScript · manual verification", code: TS_MANUAL },
        ],
      },
      {
        title: "Verify in PHP",
        body: "Same semantics with hash_equals. The raw body in PHP is file_get_contents('php://input') — never re-encode $_POST.",
        blocks: [{ kind: "code", label: "PHP · SDK", code: PHP_VERIFY }],
      },
      {
        title: "Rotate the secret without losing events",
        body: "POST …/rotate-secret returns the new secret (shown once) and previous_secret_expires_at. From that moment and for 24 h every delivery is signed with BOTH secrets, so you can switch your verifier's secret at any point inside the window without rejecting or losing a single event.",
        blocks: [
          { kind: "code", label: "POST /v1/webhook-subscriptions/{id}/rotate-secret", code: CURL_ROTATE },
          { kind: "resp", label: "Response — the new secret is shown exactly once", code: RESP_ROTATE },
        ],
      },
      {
        title: "After the grace window",
        body: "Past the 24 h the previous secret stops signing and is purged from the database. A receiver still pinned to the old secret will stop verifying: always rotate inside the window. Rotating again during an active grace replaces the previous secret with the current one and restarts the 24 h; the one before that is invalidated immediately.",
      },
    ],
  },
};
