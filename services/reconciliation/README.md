# Reconciliation Service

Motor de conciliación de LedgerCore contra fuentes externas (bancos, PSPs, proveedores de payout o cargas manuales). Compara los extractos importados con un **espejo local** de los asientos del ledger y registra las discrepancias para su triaje.

| Atributo | Valor |
|---|---|
| Módulo Go | `github.com/ASanchezT85/ledgercore/services/reconciliation` |
| Puerto | **8083** |
| Schema Postgres | **`recon`** (jamás toca schemas ajenos) |
| Eventos que consume | `ledger.transaction.posted` |
| Eventos que emite | `recon.discrepancy.detected` (vía outbox transaccional) |

## Principio de diseño: espejo por eventos

Este servicio **NO consulta el schema del ledger** (cruzar schemas está prohibido en la plataforma). Mantiene la tabla `recon.ledger_entries_mirror` alimentada exclusivamente por eventos NATS JetStream (`ledger.transaction.posted`) — patrón *event-carried state transfer*:

- Consumer **durable** (`recon-mirror`): sobrevive reinicios y retoma desde el último evento confirmado.
- **Idempotente**: el id de cada fila del espejo es un UUIDv5 determinístico derivado de `(transaction_id, índice del posting)`, y el insert es `ON CONFLICT (id) DO NOTHING`. Las redeliveries no duplican nada.
- La columna `reference` del espejo guarda la `idempotency_key` de la transacción; es la llave contra la que se cruza el `external_ref` de los extractos.
- Si `LEDGERCORE_NATS_URL` está vacía, el servicio lo registra en el log y arranca sin consumer ni poller (solo API).

## Arquitectura (hexagonal)

```
cmd/reconciliation/          # main: wiring, señales, shutdown
internal/
├── domain/                  # NÚCLEO PURO (sin I/O): entidades, parser CSV, matcher naive
├── app/                     # casos de uso + puertos (Store/TxStore); tx por caso de uso
├── adapters/
│   ├── httpapi/             # REST con net/http ServeMux (method patterns)
│   ├── postgres/            # implementación del puerto + migraciones goose embebidas + outbox
│   └── natsconsumer/        # consumer JetStream que alimenta el espejo
├── outbox/                  # poller que drena el outbox hacia NATS
└── config/                  # carga de LEDGERCORE_*
```

Toda mutación de un caso de uso (import, run) ocurre dentro de **una sola transacción por tenant** (`pgxutil.WithTenantTx` hace `SET LOCAL app.tenant_id`, con lo que aplican las políticas RLS). Las discrepancias, los contadores del run y los eventos del outbox se confirman o revierten juntos.

## Multi-tenant

Todas las tablas de negocio llevan `tenant_id UUID NOT NULL` con Row-Level Security (`ENABLE` + `FORCE`) y política `tenant_isolation`. Excepción deliberada: `recon.outbox` **no tiene RLS** porque el poller la drena entre tenants dentro de una transacción de sistema (`pgxutil.WithSystemTx`); es infraestructura, no una tabla de negocio.

## API

Auth: JWT EdDSA validado contra el JWKS de identity (`ident.RequireAuth`). En dev, con `LEDGERCORE_AUTH_DISABLED=true` el tenant se toma del header `X-Tenant-Id`.

| Método | Ruta | Descripción |
|---|---|---|
| POST | `/v1/reconciliation/sources` | Registra una fuente `{name, kind}` con `kind ∈ bank\|psp\|provider\|manual` |
| GET | `/v1/reconciliation/sources` | Lista fuentes del tenant |
| POST | `/v1/reconciliation/imports` | `multipart/form-data` con `source_id` y `file` (CSV, máx. 10 MiB) |
| POST | `/v1/reconciliation/runs` | `{source_id}` — ejecuta el matcher síncronamente |
| GET | `/v1/reconciliation/runs/{id}` | Detalle de un run |
| GET | `/v1/reconciliation/reports` | Resumen por fuente: matched/unmatched/disputed + discrepancias abiertas |
| GET | `/v1/reconciliation/discrepancies?status=` | Lista discrepancias (filtro opcional `open\|investigating\|resolved`) |
| PATCH | `/v1/reconciliation/discrepancies/{id}` | `{status, resolution_note}` — la nota se guarda en `details.resolution_note` |
| GET | `/healthz` | Liveness (sin auth) |
| GET | `/readyz` | Readiness: ping a Postgres (sin auth) |

Errores con el contrato de plataforma: `{"error":{"code","message"}}`.

> **Nota de contrato:** `contracts/openapi/reconciliation.v1.yaml` describe rutas y columnas distintas (`/v1/sources`, CSV con `external_id,...`). Este servicio implementa la especificación de la asignación (rutas bajo `/v1/reconciliation/*` y CSV `external_ref,amount,asset,occurred_at`); el OpenAPI debe actualizarse en una pasada de contratos posterior.

### Formato del CSV

Cabecera obligatoria (orden libre, case-insensitive): `external_ref,amount,asset,occurred_at`.

```csv
external_ref,amount,asset,occurred_at
bank-tx-001,10.50,USD,2026-07-24T12:05:00Z
bank-tx-002,-3.07,USD,2026-07-24T13:00:00Z
```

- `amount` es decimal y se convierte a **enteros en unidades mínimas** con `money.ParseUnits` y **exponente 2 por defecto** (`10.50` → `1050`). Nunca floats. (v2: exponente por asset desde un registro de assets.)
- `occurred_at` en RFC 3339; se normaliza a UTC.
- El primer error de fila aborta el import: se crea el registro con `status=failed` y el detalle (fila y causa) en `error`; no se inserta ninguna transacción externa.

## Matcher v1 (NAIVE, documentado como tal)

`POST /v1/reconciliation/runs` ejecuta `domain.MatchNaive` sobre las externas `unmatched` de la fuente contra todo el espejo del tenant:

| Caso | Resultado |
|---|---|
| `external_ref == reference` del espejo y mismo `amount` y `asset` | `matched` (cada asiento del espejo se consume una sola vez) |
| Mismo `external_ref` pero difiere `amount` o `asset` | discrepancia `amount_mismatch`; la externa pasa a `disputed` |
| `external_ref` sin pareja en el espejo | discrepancia `missing_internal`; la externa sigue `unmatched` |

**Dejado para v2 (a propósito):**

- **`missing_external`** (asientos del espejo sin pareja externa): el espejo aún no está acotado por fuente/cuenta, así que marcarlos hoy reportaría como faltante cada posting de otros rieles. Requiere asociar fuentes a cuentas del ledger.
- **`duplicate`**: el enum de BD ya lo contempla, pero el matcher v1 no lo produce.
- Dirección del posting (DEBIT/CREDIT), ventanas de fecha y tolerancias: ignoradas en v1.

Por cada discrepancia se inserta —en la misma transacción— un evento `recon.discrepancy.detected` en `recon.outbox`; el poller (goroutine, cada 1 s, `FOR UPDATE SKIP LOCKED`) lo publica en JetStream y marca `published_at`. El mapeo de kinds hacia el contrato de eventos es `missing_internal → missing_in_ledger`, `missing_external → missing_in_source`, `amount_mismatch → amount_mismatch`.

## Configuración

Ver `.env.example`. Variables: `LEDGERCORE_HTTP_ADDR` (por defecto `:8083`), `LEDGERCORE_DATABASE_URL` (obligatoria), `LEDGERCORE_NATS_URL`, `LEDGERCORE_JWKS_URL` (obligatoria salvo auth deshabilitada), `LEDGERCORE_AUTH_DISABLED`, `LEDGERCORE_AUTO_MIGRATE`. OpenTelemetry con las variables estándar `OTEL_*` (sin definir = no-op).

Las migraciones goose van **embebidas** (`embed.FS`) y corren al arrancar si `LEDGERCORE_AUTO_MIGRATE=true`; la tabla de versión de goose vive en `recon.goose_db_version`.

## Desarrollo

```bash
cd services/reconciliation

# Verificación completa
go mod tidy && go build ./... && go vet ./... && go test ./...

# Tests de integración (requieren Postgres; se saltan sin esta variable)
LEDGERCORE_TEST_DATABASE_URL='postgres://ledgercore_app:ledgercore_dev@localhost:5432/ledgercore?sslmode=disable' \
  go test ./internal/adapters/postgres/

# Arrancar en local
LEDGERCORE_AUTH_DISABLED=true LEDGERCORE_AUTO_MIGRATE=true \
LEDGERCORE_DATABASE_URL='postgres://ledgercore_app:ledgercore_dev@localhost:5432/ledgercore?sslmode=disable' \
  go run ./cmd/reconciliation
```

Prueba rápida (auth deshabilitada):

```bash
TENANT=1f0a6a2e-9c1b-7f2e-8a11-3c5d7e9f0b21

# 1. Fuente
curl -s -X POST localhost:8083/v1/reconciliation/sources \
  -H "X-Tenant-Id: $TENANT" -H 'Content-Type: application/json' \
  -d '{"name":"chase-operating","kind":"bank"}'

# 2. Import CSV
curl -s -X POST localhost:8083/v1/reconciliation/imports \
  -H "X-Tenant-Id: $TENANT" \
  -F "source_id=<id de la fuente>" -F "file=@statement.csv"

# 3. Run + reporte
curl -s -X POST localhost:8083/v1/reconciliation/runs \
  -H "X-Tenant-Id: $TENANT" -H 'Content-Type: application/json' \
  -d '{"source_id":"<id de la fuente>"}'
curl -s localhost:8083/v1/reconciliation/reports -H "X-Tenant-Id: $TENANT"
```

## Docker

El contexto de build es la **raíz del monorepo** (necesita `libs/go`):

```bash
docker build -f services/reconciliation/Dockerfile -t ledgercore/reconciliation .
```

Multi-stage `golang:1.26-alpine` → `gcr.io/distroless/static-debian12:nonroot`.
