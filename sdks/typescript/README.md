# @ledgercore/sdk

Español · [English below](#english)

SDK oficial de TypeScript para la API de LedgerCore (ledger de doble partida como servicio). Node 18+ y navegadores, `fetch` nativo, **cero dependencias en runtime**.

## Instalación

```bash
npm install @ledgercore/sdk
```

## Quickstart (5 líneas)

```ts
import { LedgerCore, Money } from "@ledgercore/sdk";

const lc = new LedgerCore({ apiKey: "lk_sandbox_...", baseUrl: "http://localhost:8080" });
const ledger = await lc.ledgers.create({ name: "main" });
const wallet = await lc.accounts.create({ ledger_id: ledger.id, name: "customer:42:wallet", type: "liability", normal_balance: "CREDIT" });
const txn = await lc.transactions.create({ ledger_id: ledger.id, postings: [/* débitos = créditos */] });
```

### Autenticación

Le pasas tu API key (`lk_sandbox_...` / `lk_live_...`) y el SDK gestiona todo lo demás: la intercambia por un JWT EdDSA de 15 minutos (`POST /v1/auth/token`), lo cachea, lo renueva solo cuando quedan <60 s de validez y reintenta una única vez con token fresco si la API responde 401.

### Dinero

Los montos son **siempre strings** de unidades menores (int64), nunca floats:

```ts
Money.fromDecimal("100.50", "USD", 2); // { asset: "USD", amount: "10050" }
Money.toDecimal("10050", 2);           // "100.50"
```

`fromDecimal` lanza si el decimal tiene más cifras que el exponente: el dinero no se redondea en silencio.

### Idempotencia

`transactions.create` acepta `idempotencyKey` de primera clase; si no lo pasas, el SDK genera un UUID v4. Repetir la misma key devuelve la transacción original (nunca se duplica dinero).

### Webhooks

```ts
const ok = await lc.webhooks.verifySignature(rawBody, req.headers["x-ledgercore-signature"], "whsec_...");
```

Verifica el header `X-LedgerCore-Signature` (`t=<unix>,v1=<hmac-sha256 hex>`) en tiempo constante, con ventana anti-replay de 5 minutos. El header puede traer **múltiples `v1`** (rotación de secreto: 24 h de gracia hasta `previous_secret_expires_at`, que devuelve `rotateSecret`); la verificación acepta si CUALQUIERA coincide. Usa siempre el cuerpo crudo, no JSON re-serializado. La superficie completa: `lc.webhooks.create/list/listAll/get/update/rotateSecret` y `lc.webhooks.deliveries.list/listAll/retry`.

### Paginación

Todas las colecciones responden `{ data, next_cursor }` (default 50 por página, máx 200; `next_cursor` es `null` en la última página; cursor malformado → 400 `invalid_cursor`):

```ts
let cursor: string | undefined;
do {
  const page = await lc.transactions.list({ limit: 100, cursor });
  cursor = page.next_cursor ?? undefined;
} while (cursor);

// o autopaginación:
for await (const txn of lc.transactions.listAll({ ledgerId })) { /* ... */ }
```

### Errores

Toda respuesta no-2xx lanza `LedgerCoreError` con `status`, `code` y `requestId` (de `error.request_id`, siempre presente). Catálogo estable (exportado como `ERROR_CODES` / tipo `ErrorCode`): `validation_failed`, `invalid_cursor`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `idempotency_conflict`, `unbalanced_transaction`, `insufficient_funds`, `rate_limited`, `service_unavailable`, `internal`. Credenciales rechazadas → `AuthenticationError`; red caída → `ConnectionError`.

### Superficie completa

`ledgers.create/list/listAll/get` · `accounts.create/list/listAll/get/balances` · `transactions.create/list/listAll/get/post/reverse` · `statements.get` · `trialBalance.get({ ledgerId, asOf? })` · `providerPositions.get` · `webhooks.*`

## Desarrollo

```bash
npm install && npm test && npm run build   # vitest + tsup (ESM+CJS+d.ts)
npx tsx examples/quickstart.ts             # smoke E2E contra el stack local
```

---

## English

Official TypeScript SDK for the LedgerCore API (double-entry ledger as a service). Node 18+ and browsers, native `fetch`, **zero runtime dependencies**.

```bash
npm install @ledgercore/sdk
```

```ts
import { LedgerCore, Money } from "@ledgercore/sdk";

const lc = new LedgerCore({ apiKey: "lk_sandbox_..." });
const ledger = await lc.ledgers.create({ name: "main" });
const wallet = await lc.accounts.create({ ledger_id: ledger.id, name: "customer:42:wallet", type: "liability", normal_balance: "CREDIT" });
const txn = await lc.transactions.create({ ledger_id: ledger.id, postings: [/* debits = credits */] });
```

- **Auth is handled for you**: the API key is exchanged for a 15-minute EdDSA JWT, cached, renewed under 60 s of remaining validity, and retried once on 401.
- **Money is always strings** in minor units: `Money.fromDecimal("100.50", "USD", 2)` → `"10050"`; `Money.toDecimal` goes back. No silent rounding.
- **Idempotency first-class**: pass `idempotencyKey` to `transactions.create` or let the SDK generate a UUID v4; retries never duplicate money.
- **Pagination everywhere**: every collection answers `{ data, next_cursor }` (default 50, max 200, `next_cursor` null on the last page); `list({ limit, cursor })` plus `listAll()` auto-pagination on every collection.
- **Webhooks**: `lc.webhooks.verifySignature(rawBody, header, secret)` verifies `X-LedgerCore-Signature` in constant time with a 5-minute replay window; multiple `v1` entries (secret rotation, `previous_secret_expires_at` grace) verify if ANY matches.
- **Typed errors**: non-2xx → `LedgerCoreError { status, code, requestId }` with a stable code catalog (`ERROR_CODES`: `validation_failed`, `invalid_cursor`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `idempotency_conflict`, `unbalanced_transaction`, `insufficient_funds`, `rate_limited`, `service_unavailable`, `internal`); bad credentials → `AuthenticationError`; network → `ConnectionError`.

Full surface: `ledgers`, `accounts`, `transactions` (create/list/get/post/reverse), `statements.get`, `trialBalance.get`, `providerPositions.get`, `webhooks` (subscriptions CRUD, deliveries, signature verification).
