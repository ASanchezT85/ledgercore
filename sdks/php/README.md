# ledgercore/sdk (PHP)

Español · [English below](#english)

SDK oficial de PHP para la API de LedgerCore (ledger de doble partida como servicio). PHP 8.1+, cURL nativo, **cero dependencias obligatorias en runtime** (el transporte es una interfaz: puedes enchufar tu propio cliente PSR-18).

## Instalación

```bash
composer require ledgercore/sdk
```

## Quickstart (5 líneas)

```php
use LedgerCore\LedgerCore;
use LedgerCore\Money;

$lc = new LedgerCore(['api_key' => 'lk_sandbox_...', 'base_url' => 'http://localhost:8080']);
$ledger = $lc->ledgers->create(['name' => 'main']);
$wallet = $lc->accounts->create(['ledger_id' => $ledger['id'], 'name' => 'customer:42:wallet', 'type' => 'liability', 'normal_balance' => 'CREDIT']);
$txn = $lc->transactions->create(['ledger_id' => $ledger['id'], 'postings' => [/* débitos = créditos */]]);
```

### Autenticación

Le pasas tu API key (`lk_sandbox_...` / `lk_live_...`) y el SDK gestiona todo lo demás: la intercambia por un JWT EdDSA de 15 minutos (`POST /v1/auth/token`), lo cachea, lo renueva solo cuando quedan <60 s de validez y reintenta una única vez con token fresco si la API responde 401.

### Dinero

Los montos son **siempre strings** de unidades menores (int64), nunca floats:

```php
Money::fromDecimal('100.50', 'USD', 2); // ['asset' => 'USD', 'amount' => '10050']
Money::toDecimal('10050', 2);           // "100.50"
```

`fromDecimal` lanza si el decimal tiene más cifras que el exponente: el dinero no se redondea en silencio.

### Idempotencia

`transactions->create` acepta `idempotency_key` de primera clase; si no lo pasas, el SDK genera un UUID v4. Repetir la misma key devuelve la transacción original (nunca se duplica dinero).

### Webhooks

```php
use LedgerCore\Webhook;

$ok = Webhook::verifySignature($rawBody, $_SERVER['HTTP_X_LEDGERCORE_SIGNATURE'] ?? null, 'lcwh_...');
```

Verifica el header `X-LedgerCore-Signature` (`t=<unix>,v1=<hmac-sha256 hex>`) en tiempo constante (`hash_equals`), con ventana anti-replay de 5 minutos. El header puede traer **múltiples `v1`** (rotación de secreto: 24 h de gracia hasta `previous_secret_expires_at`, que devuelve `rotateSecret`); la verificación acepta si CUALQUIERA coincide. Usa siempre el cuerpo crudo (`file_get_contents('php://input')`), no JSON re-serializado. La superficie completa: `$lc->webhooks->create/list/listAll/get/update/rotateSecret/deliveries/deliveriesAll/retryDelivery`.

### Paginación

Todas las colecciones responden `['data' => [...], 'next_cursor' => ?string]` (default 50 por página, máx 200; `next_cursor` es `null` en la última página; cursor malformado → 400 `invalid_cursor`):

```php
$cursor = null;
do {
    $page = $lc->transactions->list(limit: 100, cursor: $cursor);
    $cursor = $page['next_cursor'];
} while ($cursor !== null);

// o autopaginación:
foreach ($lc->transactions->listAll($ledgerId) as $txn) { /* ... */ }
```

### Errores

Toda respuesta no-2xx lanza `LedgerCore\Exception\LedgerCoreException` con `status`, `errorCode` y `requestId` (de `error.request_id`, siempre presente). Catálogo estable en `LedgerCore\ErrorCode` (`ErrorCode::ALL`): `validation_failed`, `invalid_cursor`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `idempotency_conflict`, `unbalanced_transaction`, `insufficient_funds`, `rate_limited`, `service_unavailable`, `internal`. Credenciales rechazadas → `AuthenticationException`; red caída → `ConnectionException`.

### Superficie completa

`ledgers` create/list/listAll/get · `accounts` create/list/listAll/get/balances · `transactions` create/list/listAll/get/post/reverse · `statements->get` · `trialBalance->get($ledgerId, $asOf = null)` · `providerPositions->get` · `webhooks->*`

## Desarrollo

```bash
composer install && composer test    # PHPUnit
php examples/quickstart.php          # smoke E2E contra el stack local
```

---

## English

Official PHP SDK for the LedgerCore API (double-entry ledger as a service). PHP 8.1+, native cURL, **zero mandatory runtime dependencies** (the transport is an interface — plug in your own PSR-18 client if you prefer).

```bash
composer require ledgercore/sdk
```

```php
use LedgerCore\LedgerCore;
use LedgerCore\Money;

$lc = new LedgerCore(['api_key' => 'lk_sandbox_...']);
$ledger = $lc->ledgers->create(['name' => 'main']);
$wallet = $lc->accounts->create(['ledger_id' => $ledger['id'], 'name' => 'customer:42:wallet', 'type' => 'liability', 'normal_balance' => 'CREDIT']);
$txn = $lc->transactions->create(['ledger_id' => $ledger['id'], 'postings' => [/* debits = credits */]]);
```

- **Auth is handled for you**: the API key is exchanged for a 15-minute EdDSA JWT, cached, renewed under 60 s of remaining validity, and retried once on 401.
- **Money is always strings** in minor units: `Money::fromDecimal('100.50', 'USD', 2)` → `"10050"`; `Money::toDecimal` goes back. No silent rounding.
- **Idempotency first-class**: pass `idempotency_key` to `transactions->create` or let the SDK generate a UUID v4; retries never duplicate money.
- **Pagination everywhere**: every collection answers `['data', 'next_cursor']` (default 50, max 200, `next_cursor` null on the last page); `list($limit, $cursor)` plus `listAll()` generators on every collection.
- **Webhooks**: `Webhook::verifySignature($rawBody, $header, $secret)` verifies `X-LedgerCore-Signature` in constant time with a 5-minute replay window; multiple `v1` entries (secret rotation, `previous_secret_expires_at` grace) verify if ANY matches.
- **Typed exceptions**: non-2xx → `LedgerCoreException { status, errorCode, requestId }` with a stable code catalog in `LedgerCore\ErrorCode` (`validation_failed`, `invalid_cursor`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `idempotency_conflict`, `unbalanced_transaction`, `insufficient_funds`, `rate_limited`, `service_unavailable`, `internal`); bad credentials → `AuthenticationException`; network → `ConnectionException`.

Full surface: `ledgers`, `accounts`, `transactions` (create/list/get/post/reverse), `statements->get`, `trialBalance->get`, `providerPositions->get`, `webhooks` (subscriptions CRUD, deliveries, signature verification).
