import type { GuideContent } from "../../docs-ui";
import { API_BASE } from "../../docs-ui";

const CURL_TOKEN = `export API=${API_BASE}

TOKEN=$(curl -s -X POST $API/v1/auth/token \\
  -H "Content-Type: application/json" \\
  -d '{ "api_key": "lk_sandbox_TU_LLAVE" }' | jq -r .access_token)`;

const RESP_TOKEN = `{
  "access_token": "eyJhbGciOiJFZERTQSIsImtpZCI6ImtleS0xIn0...",
  "token_type": "Bearer",
  "expires_in": 900
}`;

const CURL_USE = `curl -s $API/v1/ledgers \\
  -H "Authorization: Bearer $TOKEN" | jq`;

const TS_AUTH = `import { LedgerCore } from "@ledgercore/sdk";

// El SDK gestiona todo el ciclo: intercambia la API key por el JWT,
// lo cachea, lo renueva cuando quedan <60 s y reintenta una vez en 401.
const lc = new LedgerCore({
  apiKey: "lk_sandbox_...",
  baseUrl: "${API_BASE}",
});

const ledgers = await lc.ledgers.list();`;

const PHP_AUTH = `use LedgerCore\\LedgerCore;

// Mismo comportamiento: token exchange, caché, renovación <60 s,
// un retry con token fresco si la API responde 401.
$lc = new LedgerCore([
    'api_key'  => 'lk_sandbox_...',
    'base_url' => '${API_BASE}',
]);

$ledgers = $lc->ledgers->list();`;

export const GUIDE: GuideContent = {
  es: {
    badge: "Guía · Autenticación",
    title: "Autenticación: de la API key al JWT",
    subtitle:
      "La API no acepta la API key directamente en cada request: la intercambias por un JWT EdDSA de 15 minutos y ese token es el que viaja en Authorization: Bearer. Los SDKs hacen todo el ciclo por ti.",
    backLabel: "Docs",
    copy: "Copiar",
    copied: "Copiado",
    sections: [
      {
        title: "Tu API key es la credencial raíz",
        body: "Al crear tu sandbox recibes una llave lk_sandbox_… (lk_live_… en producción) que se muestra una sola vez. Guárdala en un secreto del servidor: nunca en el frontend, nunca en git. Si se compromete, revócala y emite otra.",
      },
      {
        title: "Intercambia la llave por un token",
        body: "POST /v1/auth/token devuelve un JWT firmado con EdDSA (Ed25519) que expira en 900 segundos (15 minutos). El campo expires_in te dice cuánto le queda. Los claims incluyen tenant_id, env (sandbox | live) y scopes (hoy siempre ledger:read y ledger:write).",
        blocks: [
          { kind: "code", label: "POST /v1/auth/token", code: CURL_TOKEN },
          { kind: "resp", label: "Respuesta esperada", code: RESP_TOKEN },
        ],
      },
      {
        title: "Usa el token en cada request",
        body: "Todos los endpoints (salvo el propio token exchange y el JWKS público) esperan Authorization: Bearer <token>. El tenant se deriva del JWT: cada recurso queda aislado a tu tenant automáticamente.",
        blocks: [{ kind: "code", label: "Request autenticado", code: CURL_USE }],
      },
      {
        title: "Expiración y renovación",
        body: "El token dura 15 minutos. Cuando expira, la API responde 401 unauthorized: vuelve a hacer el exchange y reintenta. Regla práctica: renueva proactivamente cuando queden menos de 60 segundos de validez — es exactamente lo que hacen los SDKs.",
      },
      {
        title: "Con el SDK de TypeScript",
        body: "Le pasas la API key una vez y el SDK gestiona intercambio, caché, renovación (<60 s de validez restante) y un único retry con token fresco si la API responde 401.",
        blocks: [{ kind: "code", label: "TypeScript · @ledgercore/sdk", code: TS_AUTH }],
      },
      {
        title: "Con el SDK de PHP",
        body: "Idéntico comportamiento en PHP 8.1+: el token nunca lo tocas tú.",
        blocks: [{ kind: "code", label: "PHP · ledgercore/sdk", code: PHP_AUTH }],
      },
    ],
  },
  en: {
    badge: "Guide · Authentication",
    title: "Authentication: from API key to JWT",
    subtitle:
      "The API does not accept the API key directly on every request: you exchange it for a 15-minute EdDSA JWT and that token travels in Authorization: Bearer. The SDKs run the whole cycle for you.",
    backLabel: "Docs",
    copy: "Copy",
    copied: "Copied",
    sections: [
      {
        title: "Your API key is the root credential",
        body: "When you create your sandbox you receive an lk_sandbox_… key (lk_live_… in production) shown exactly once. Keep it in a server-side secret: never in the frontend, never in git. If it leaks, revoke it and issue a new one.",
      },
      {
        title: "Exchange the key for a token",
        body: "POST /v1/auth/token returns an EdDSA-signed (Ed25519) JWT that expires in 900 seconds (15 minutes). The expires_in field tells you how long it has left. Claims include tenant_id, env (sandbox | live) and scopes (currently always ledger:read and ledger:write).",
        blocks: [
          { kind: "code", label: "POST /v1/auth/token", code: CURL_TOKEN },
          { kind: "resp", label: "Expected response", code: RESP_TOKEN },
        ],
      },
      {
        title: "Use the token on every request",
        body: "Every endpoint (except the token exchange itself and the public JWKS) expects Authorization: Bearer <token>. The tenant is derived from the JWT: every resource is automatically scoped to your tenant.",
        blocks: [{ kind: "code", label: "Authenticated request", code: CURL_USE }],
      },
      {
        title: "Expiry and renewal",
        body: "The token lives 15 minutes. When it expires the API answers 401 unauthorized: run the exchange again and retry. Rule of thumb: renew proactively when fewer than 60 seconds of validity remain — exactly what the SDKs do.",
      },
      {
        title: "With the TypeScript SDK",
        body: "Pass the API key once and the SDK handles the exchange, caching, renewal (<60 s of remaining validity) and a single retry with a fresh token if the API answers 401.",
        blocks: [{ kind: "code", label: "TypeScript · @ledgercore/sdk", code: TS_AUTH }],
      },
      {
        title: "With the PHP SDK",
        body: "Identical behavior on PHP 8.1+: you never touch the token yourself.",
        blocks: [{ kind: "code", label: "PHP · ledgercore/sdk", code: PHP_AUTH }],
      },
    ],
  },
};
