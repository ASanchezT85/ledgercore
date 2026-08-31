# Servicio Webhooks

Despachador de webhooks firmados de LedgerCore: consume los eventos de la
plataforma desde NATS JetStream y los entrega, con firma HMAC verificable,
a los endpoints registrados por cada tenant.

- **Módulo:** `github.com/ASanchezT85/ledgercore/services/webhooks`
- **Puerto:** `8084`
- **Schema Postgres:** `webhooks` (search_path fijado; RLS por `tenant_id`)
- **Consume:** `ledger.>` y `recon.>` (stream `LEDGERCORE`, consumers durables
  `webhooks-ledger` y `webhooks-recon`)

## Arquitectura (hexagonal)

```
cmd/webhooks/            arranque y cableado
internal/domain/         modelo puro: matching de event_types, política de
                         reintentos/backoff, validación de URL, secretos
internal/signature/      firma HMAC (referencia para los SDK de clientes)
internal/app/            casos de uso + puertos de almacenamiento
internal/dispatcher/     worker de entrega (loop 1s, lotes de 50)
internal/adapters/
  postgres/              repositorio pgx + migraciones goose embebidas
  natsconsumer/          consumer JetStream durable
  httpapi/               API REST /v1 + /healthz /readyz
```

## Flujo de un evento

1. Un servicio publica un envelope en JetStream (vía su outbox).
2. El consumer lo recibe, busca las suscripciones **activas** del tenant cuyo
   `event_types` contenga el tipo (o `"*"`) y crea filas `deliveries` en
   estado `pending`. La inserción es idempotente por
   `UNIQUE (subscription_id, event_id)`, así que las redelivery de NATS no
   duplican nada.
3. El dispatcher (cada 1 s) toma hasta 50 deliveries vencidas con
   `FOR UPDATE SKIP LOCKED`, les aplica un *lease* de 2 minutos (si el worker
   muere, reaparecen solas) y hace el POST en paralelo.
4. Respuesta 2xx → `delivered`. Cualquier otra cosa → `attempts+1` y backoff
   `1m, 5m, 30m, 2h, 12h`; al llegar a **5 intentos** pasa a `dead`.

Estados: `pending` (en cola) → `delivered`, o `failed` (reintentando) →
`dead`. El retry manual re-encola `failed`/`dead` como `pending` con el
contador de intentos en cero. Desactivar una suscripción pausa sus entregas
pendientes (el claim las ignora hasta reactivarla).

Semántica **al-menos-una-vez**: si el proceso muere entre el POST exitoso y
la escritura de `delivered`, el evento se reenvía al expirar el lease. Los
receptores deben deduplicar por `X-LedgerCore-Event-Id`.

## Firma de los webhooks

Cada POST lleva:

| Header | Contenido |
|---|---|
| `Content-Type` | `application/json` |
| `X-LedgerCore-Event-Id` | UUID del evento (clave de deduplicación) |
| `X-LedgerCore-Event-Type` | p. ej. `ledger.transaction.posted` |
| `X-LedgerCore-Signature` | `t=<unix>,v1=<hex hmac-sha256(secret, t + "." + body)>` |

El paquete `internal/signature` (`Sign`, `Header`, `Verify`) es la
**implementación de referencia para los SDK**: comparación en tiempo
constante, tolerancia de timestamp recomendada de 5 minutos y soporte de
varias entradas `v1` para ventanas de rotación de secreto. Los tests fijan
vectores conocidos calculados con una implementación independiente.

El secreto tiene la forma `lcwh_<32 base62>` y **solo se devuelve al crear
la suscripción o al rotarlo**; después no es legible por API.
`subscriptions.secret` queda como TEXT con un TODO de cifrado vía KMS.

## API (`/v1`, autenticada)

Autenticación: JWT EdDSA contra el JWKS de identity. Con
`LEDGERCORE_AUTH_DISABLED=true` (solo dev) el tenant sale del header
`X-Tenant-Id`.

| Método y ruta | Descripción |
|---|---|
| `POST /v1/webhook-subscriptions` | Crea suscripción `{url, event_types}`. En entorno **live** la URL debe ser `https`. Devuelve el `secret` una única vez. |
| `GET /v1/webhook-subscriptions` | Lista las suscripciones del tenant (sin secreto). |
| `GET /v1/webhook-subscriptions/{id}` | Detalle (sin secreto). |
| `PATCH /v1/webhook-subscriptions/{id}` | Patch parcial `{url?, event_types?, active?}`. |
| `POST /v1/webhook-subscriptions/{id}/rotate-secret` | Genera y devuelve el secreto nuevo una única vez. |
| `GET /v1/webhook-deliveries?status=&subscription_id=&cursor=&limit=` | Listado paginado por keyset (cursor opaco `next_cursor`). |
| `POST /v1/webhook-deliveries/{id}/retry` | Re-encola una delivery `failed` o `dead` (409 en otros estados). |

`event_types` acepta los topics canónicos de `libs/go/events` o `"*"`.
Errores con el contrato estándar `{"error":{"code","message"}}`.

Salud: `GET /healthz` (liveness) y `GET /readyz` (ping a Postgres).

### Ejemplo (dev, auth deshabilitada)

```bash
curl -s -X POST localhost:8084/v1/webhook-subscriptions \
  -H "X-Tenant-Id: 6f1f8a3e-3f2b-4a3e-9a44-1f2d3c4b5a69" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://cliente.example.com/hooks","event_types":["ledger.transaction.posted","recon.discrepancy.detected"]}'
```

## Base de datos

Migración goose embebida `0001_init.sql` (corre al arrancar si
`LEDGERCORE_AUTO_MIGRATE=true`):

- `subscriptions(id, tenant_id, url, secret, event_types TEXT[], active, created_at)`
- `deliveries(id, tenant_id, subscription_id, event_id, event_type, payload JSONB,
  status, attempts, next_attempt_at, last_status_code, last_error, created_at,
  delivered_at)` con índice `(status, next_attempt_at)` y
  `UNIQUE (subscription_id, event_id)`

Ambas tablas con RLS (`tenant_isolation` sobre `app.tenant_id`). Todo acceso
de negocio pasa por `pgxutil.WithTenantTx`; la única excepción es el *claim*
del dispatcher, que recorre todos los tenants en una transacción de sistema
(en producción ese rol debe ser dueño de las tablas o tener `BYPASSRLS`).

## Variables de entorno

Ver `.env.example`. Resumen: `LEDGERCORE_HTTP_ADDR` (`:8084`),
`LEDGERCORE_DATABASE_URL` (obligatoria), `LEDGERCORE_NATS_URL`,
`LEDGERCORE_JWKS_URL` (obligatoria salvo auth deshabilitada),
`LEDGERCORE_AUTH_DISABLED`, `LEDGERCORE_AUTO_MIGRATE` y las `OTEL_*`
estándar (sin definir = telemetría no-op).

## Desarrollo

```bash
cd services/webhooks
go mod tidy && go build ./... && go vet ./... && go test ./...
go run ./cmd/webhooks
```

Los tests unitarios (firma con vectores fijos, backoff, matching, dispatcher
contra `httptest`) no necesitan infraestructura. El test de integración del
repositorio solo corre si `LEDGERCORE_TEST_DATABASE_URL` está definida; en
caso contrario se salta.

Imagen Docker (contexto = raíz del repo):

```bash
docker build -f services/webhooks/Dockerfile -t ledgercore/webhooks .
```
