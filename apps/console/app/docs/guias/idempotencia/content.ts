import type { GuideContent } from "../../docs-ui";

const CURL_FIRST = `curl -si -X POST $API/v1/transactions \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d "{ \\"ledger_id\\": \\"$LEDGER_ID\\", \\"idempotency_key\\": \\"dep-2026-0001\\",
        \\"postings\\": [ /* débitos = créditos */ ] }"`;

const RESP_FIRST = `HTTP/2 201
{ "id": "3c2d8e0f-…", "status": "posted", "idempotency_key": "dep-2026-0001", … }`;

const RESP_REPLAY = `HTTP/2 200
x-idempotent-replay: true

{ "id": "3c2d8e0f-…", "status": "posted", "idempotency_key": "dep-2026-0001", … }`;

const TS_IDEM = `// Key explícita (recomendado: derivada de tu operación de negocio)
const txn = await lc.transactions.create(
  { ledger_id, postings },
  { idempotencyKey: "dep-2026-0001" },
);

// Sin key: el SDK genera un UUID v4 por llamada.
// Un retry DEL SDK reutiliza esa misma key — nunca se duplica dinero.`;

const PHP_IDEM = `// Key explícita
$txn = $lc->transactions->create([
    'ledger_id'       => $ledgerId,
    'idempotency_key' => 'dep-2026-0001',
    'postings'        => $postings,
]);

// Sin key: el SDK genera un UUID v4 por llamada y lo reutiliza en sus retries.`;

export const GUIDE: GuideContent = {
  es: {
    badge: "Guía · Idempotencia",
    title: "Idempotencia: los reintentos nunca duplican dinero",
    subtitle:
      "Cada transacción lleva una idempotency_key. Repetir el mismo POST con la misma key devuelve la transacción original con la cabecera X-Idempotent-Replay: true — la garantía que hace seguros los retries de red.",
    backLabel: "Docs",
    copy: "Copiar",
    copied: "Copiado",
    sections: [
      {
        title: "El problema que resuelve",
        body: "Un timeout de red no te dice si el servidor procesó tu POST o no. Sin idempotencia, reintentar puede duplicar un movimiento de dinero; no reintentar puede perderlo. Con idempotency_key el reintento es siempre seguro: o crea la transacción o te devuelve la que ya existía.",
      },
      {
        title: "Primera llamada: 201 Created",
        body: "Envía la transacción con una idempotency_key elegida por ti — idealmente derivada de la operación de negocio (id de depósito, id de orden), no aleatoria por intento.",
        blocks: [
          { kind: "code", label: "POST /v1/transactions", code: CURL_FIRST },
          { kind: "resp", label: "Primera vez (201)", code: RESP_FIRST },
        ],
      },
      {
        title: "Replay: 200 + X-Idempotent-Replay: true",
        body: "El MISMO POST otra vez no crea nada: la API responde 200 con la transacción original y la cabecera X-Idempotent-Replay: true para que distingas un replay de una creación.",
        blocks: [{ kind: "resp", label: "Segunda vez (200, replay)", code: RESP_REPLAY }],
      },
      {
        title: "Semántica exacta de los reintentos",
        blocks: [
          {
            kind: "list",
            items: [
              "Misma key + mismo contenido → 200 con la respuesta original y X-Idempotent-Replay: true. Nunca se duplica dinero.",
              "Misma key con payload distinto → hoy la reutilización devuelve la respuesta original (replay); el código 409 idempotency_conflict está reservado en el catálogo para rechazar este caso. No reutilices una key con otro contenido.",
              "Tras un timeout o un 5xx: reintenta con LA MISMA key (backoff exponencial). Con 429 rate_limited el retry con la misma key también es seguro.",
              "Ante un 4xx de validación (400/422) no reintentes sin corregir: el contenido es el problema, no la red.",
            ],
          },
        ],
      },
      {
        title: "Con los SDKs",
        body: "transactions.create acepta la key de primera clase; si no la pasas, el SDK genera un UUID v4 y lo reutiliza en sus propios reintentos.",
        blocks: [
          { kind: "code", label: "TypeScript", code: TS_IDEM },
          { kind: "code", label: "PHP", code: PHP_IDEM },
        ],
      },
    ],
  },
  en: {
    badge: "Guide · Idempotency",
    title: "Idempotency: retries never duplicate money",
    subtitle:
      "Every transaction carries an idempotency_key. Repeating the same POST with the same key returns the original transaction with the X-Idempotent-Replay: true header — the guarantee that makes network retries safe.",
    backLabel: "Docs",
    copy: "Copy",
    copied: "Copied",
    sections: [
      {
        title: "The problem it solves",
        body: "A network timeout does not tell you whether the server processed your POST. Without idempotency, retrying can duplicate a money movement; not retrying can lose it. With idempotency_key the retry is always safe: it either creates the transaction or returns the one that already exists.",
      },
      {
        title: "First call: 201 Created",
        body: "Send the transaction with an idempotency_key you choose — ideally derived from the business operation (deposit id, order id), not random per attempt.",
        blocks: [
          { kind: "code", label: "POST /v1/transactions", code: CURL_FIRST },
          { kind: "resp", label: "First time (201)", code: RESP_FIRST },
        ],
      },
      {
        title: "Replay: 200 + X-Idempotent-Replay: true",
        body: "The SAME POST again creates nothing: the API answers 200 with the original transaction and the X-Idempotent-Replay: true header so you can tell a replay from a creation.",
        blocks: [{ kind: "resp", label: "Second time (200, replay)", code: RESP_REPLAY }],
      },
      {
        title: "Exact retry semantics",
        blocks: [
          {
            kind: "list",
            items: [
              "Same key + same content → 200 with the original response and X-Idempotent-Replay: true. Money is never duplicated.",
              "Same key with a different payload → today reuse returns the original response (replay); the 409 idempotency_conflict code is reserved in the catalog to reject this case. Do not reuse a key with different content.",
              "After a timeout or a 5xx: retry with the SAME key (exponential backoff). On 429 rate_limited the same-key retry is also safe.",
              "On a validation 4xx (400/422) do not retry without fixing it: the content is the problem, not the network.",
            ],
          },
        ],
      },
      {
        title: "With the SDKs",
        body: "transactions.create accepts the key first-class; if you don't pass one, the SDK generates a UUID v4 and reuses it across its own retries.",
        blocks: [
          { kind: "code", label: "TypeScript", code: TS_IDEM },
          { kind: "code", label: "PHP", code: PHP_IDEM },
        ],
      },
    ],
  },
};
