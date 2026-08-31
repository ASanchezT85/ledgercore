# ledger-core

Motor transaccional de doble entrada de LedgerCore. Es la fuente de verdad
contable: libros (ledgers), cuentas, transacciones balanceadas, holds
(autorizaciones en dos fases), balances por activo y balanza de comprobación.

- **Módulo:** `github.com/ASanchezT85/ledgercore/services/ledger-core`
- **Puerto:** `8081`
- **Schema Postgres:** `ledger` (jamás toca otros schemas)
- **Contrato API:** `contracts/openapi/ledger.v1.yaml`
- **Eventos:** `contracts/events/*.schema.json`

## Arquitectura (hexagonal)

```
cmd/server/               arranque, configuración, wiring
internal/domain/          entidades y reglas puras (sin infraestructura)
internal/app/             casos de uso + puerto de persistencia (Store)
internal/adapters/http/   handlers, vistas del contrato y router (net/http + ServeMux)
internal/adapters/postgres/  repositorios pgx, migraciones goose embebidas
internal/adapters/outbox/    poller que publica el outbox a NATS JetStream
```

Reglas centrales del dominio:

- `ValidateBalanced(postings)`: por **cada** activo, la suma de débitos es
  igual a la suma de créditos; mínimo 2 postings; montos > 0; sin overflow
  de int64. El dinero siempre es entero en unidades mínimas (`libs/go/money`),
  nunca float.
- `Reverse(tx)`: genera la transacción espejo (direcciones invertidas,
  referencia `reversal-of:<id>`) y enlaza ambas.

## Garantías

- **Atomicidad:** `CreateTransaction` escribe transacción + postings +
  `account_balances` + evento en el outbox + registro de idempotencia en la
  **misma** transacción de base de datos. Todo o nada.
- **Idempotencia:** `idempotency_key` único por tenant. Reintentos devuelven
  la respuesta original con `X-Idempotent-Replay: true` y status 200.
- **Inmutabilidad:** triggers en Postgres impiden UPDATE/DELETE de `postings`
  y solo permiten las transiciones válidas de `transactions`
  (`draft→posted`, `posted→reversed`). Escotilla de mantenimiento documentada
  en la migración: `SET ledger.allow_maintenance = '1'` (solo reparaciones
  manuales controladas).
- **Multi-tenant:** todas las tablas de negocio llevan `tenant_id` con RLS
  (`tenant_isolation`); cada operación corre dentro de
  `pgxutil.WithTenantTx`, que hace `SET LOCAL app.tenant_id`.
- **Eventos:** patrón outbox. El poller (cada 500 ms, lotes de 100 con
  `FOR UPDATE SKIP LOCKED`) publica a NATS JetStream con `MsgId` = id del
  evento, así el broker deduplica reentregas. Sin `LEDGERCORE_NATS_URL`, el
  servicio funciona y los eventos quedan acumulados en la tabla.

## Endpoints

Bajo `/v1` (autenticado con JWT EdDSA contra el JWKS de identity, o con el
header `X-Tenant-Id` si `LEDGERCORE_AUTH_DISABLED=true`):

| Método y ruta | Descripción |
|---|---|
| `POST /v1/ledgers` · `GET /v1/ledgers` · `GET /v1/ledgers/{id}` | Libros |
| `POST /v1/accounts` · `GET /v1/accounts` · `GET /v1/accounts/{id}` | Cuentas |
| `GET /v1/accounts/{id}/balances` | Balances por activo (posted/pending/available/held) |
| `GET /v1/accounts/{id}/entries` | Postings de la cuenta (paginación keyset) |
| `POST /v1/transactions` | Crear (posted por defecto, o draft) — idempotente |
| `GET /v1/transactions` · `GET /v1/transactions/{id}` | Consulta |
| `POST /v1/transactions/{id}/post` | draft → posted (idempotente si ya está posted) |
| `POST /v1/transactions/{id}/reverse` | Crea y postea la transacción espejo |
| `POST /v1/holds` · `GET /v1/holds/{id}` | Reservar fondos (verifica available) |
| `POST /v1/holds/{id}/capture` · `POST /v1/holds/{id}/release` | Ciclo de vida del hold |
| `GET /v1/trial-balance?ledger_id=...` | Balanza de comprobación con totales por activo |
| `GET /v1/statements` · `GET /v1/provider-positions` | **501** — hitos futuros |

Fuera de auth: `GET /healthz` (liveness) y `GET /readyz` (ping a Postgres).

Los montos viajan como string de int64 en unidades mínimas
(`{"asset":"USD","amount":"10000"}` = USD 100.00), según el contrato.

## Configuración

| Variable | Descripción |
|---|---|
| `LEDGERCORE_HTTP_ADDR` | Dirección de escucha (default `:8081`) |
| `LEDGERCORE_DATABASE_URL` | **Obligatoria.** Postgres 17; el pool fija `search_path=ledger` |
| `LEDGERCORE_NATS_URL` | NATS JetStream; vacía = poller del outbox apagado |
| `LEDGERCORE_JWKS_URL` | JWKS de identity para validar tokens |
| `LEDGERCORE_AUTH_DISABLED` | `true` solo en dev: tenant por header `X-Tenant-Id` |
| `LEDGERCORE_AUTO_MIGRATE` | `true` = aplica migraciones goose al arrancar |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry estándar; sin definir = no-op |

Ver `.env.example` (credenciales solo de desarrollo).

## Desarrollo

```bash
cd services/ledger-core
go mod tidy && go build ./... && go vet ./... && go test ./...
```

- Los tests de dominio no necesitan base de datos.
- Los tests de integración se saltan si `LEDGERCORE_TEST_DATABASE_URL` no
  está definida; con ella, migran el schema `ledger` y se aíslan por tenant
  (UUID nuevo por test), sin truncar tablas compartidas.

La imagen Docker es multi-stage (`golang:1.26-alpine` →
`gcr.io/distroless/static-debian12`) y se construye desde la **raíz** del
repo:

```bash
docker build -f services/ledger-core/Dockerfile -t ledgercore/ledger-core .
```

## Pendientes conocidos

- Expiración automática de holds (`expired`): el estado existe, falta el
  barrido programado que libere holds vencidos.
- `GET /v1/statements` y `GET /v1/provider-positions` devuelven 501.
- El parámetro `as_of` de la balanza de comprobación se ignora: siempre se
  calcula sobre los balances actuales.
