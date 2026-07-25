# Infraestructura local de LedgerCore

Stack de desarrollo completo con Docker Compose: PostgreSQL 17, NATS JetStream, gateway Traefik y los cuatro servicios Go. Todo lo que hay aquí es **solo para desarrollo local** — credenciales, auth deshabilitada y dashboard de Traefik incluidos.

## Requisitos

- Docker + Docker Compose v2
- (Opcional) `make` en Git Bash, o PowerShell con `scripts/dev.ps1`
- (Opcional) `jq` para los ejemplos con `curl`

## Levantar el stack

Desde la raíz del repo:

```bash
# Con make (Git Bash / Linux / macOS)
make dev        # postgres + nats + traefik + los 4 servicios
make dev-obs    # lo anterior + grafana/otel-lgtm (observabilidad)
make down       # apagar (agrega -v a mano si quieres borrar el volumen de datos)
```

```powershell
# Con PowerShell (Windows)
.\scripts\dev.ps1                 # equivale a make dev
.\scripts\dev.ps1 -Obs            # equivale a make dev-obs
.\scripts\dev.ps1 -Web            # agrega la consola web (puerto 3000)
.\scripts\dev.ps1 -Down           # apagar
.\scripts\dev.ps1 -Down -Volumes  # apagar y borrar el volumen de Postgres
```

O directamente con compose:

```bash
docker compose -f infra/compose/docker-compose.yml up -d --build
docker compose -f infra/compose/docker-compose.yml --profile web --profile obs up -d --build
```

### Perfiles

| Perfil   | Qué agrega |
|----------|------------|
| *(base)* | postgres, nats, gateway (Traefik) y los servicios ledger-core, identity, reconciliation, webhooks |
| `web`    | `console` — la consola Next.js en el puerto 3000 |
| `obs`    | `otel-lgtm` — Grafana + Tempo + Loki + Mimir todo-en-uno; para exportar trazas define `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-lgtm:4318` en los servicios (si no está definida, OTel queda en no-op) |

## Puertos

| Componente         | Puerto host | Notas |
|--------------------|-------------|-------|
| Gateway (Traefik)  | 8080        | Punto de entrada único de la API |
| Dashboard Traefik  | 8090        | Solo dev (`api.insecure`) |
| ledger-core        | 8081        | Acceso directo, sin gateway |
| identity           | 8082        | Acceso directo, sin gateway |
| reconciliation     | 8083        | Acceso directo, sin gateway |
| webhooks           | 8084        | Acceso directo, sin gateway |
| console            | 3000        | Perfil `web` |
| PostgreSQL 17      | 5432        | db `ledgercore` / user `ledgercore_app` / pass `ledgercore_dev` (solo dev) |
| NATS               | 4222        | Monitor HTTP en 8222 |
| Grafana (otel-lgtm)| 3100        | Perfil `obs`; OTLP en 4317 (gRPC) y 4318 (HTTP) |

### Enrutado del gateway (por prioridad)

| Prefijo | Servicio |
|---------|----------|
| `/v1/auth`, `/v1/tenants`, `/v1/api-keys`, `/.well-known` | identity |
| `/v1/reconciliation` | reconciliation |
| `/v1/webhook-subscriptions`, `/v1/webhook-deliveries` | webhooks |
| resto de `/v1` (prioridad más baja) | ledger-core |

## Base de datos

Una sola instancia de Postgres con **un schema por servicio**: `ledger`, `identity`, `recon`, `webhooks`. El script `infra/postgres/init/01-init.sql` crea el rol `ledgercore_app` (LOGIN, `NOBYPASSRLS` — las políticas RLS de `tenant_id` aplican siempre) y los cuatro schemas; corre una única vez, al inicializar el volumen. Las **migraciones viven en cada servicio** (goose embebido) y corren al arrancar porque el compose define `LEDGERCORE_AUTO_MIGRATE=true`.

Para reinicializar la base desde cero: `.\scripts\dev.ps1 -Down -Volumes` (o `docker compose ... down -v`) y volver a levantar.

## Bootstrap: primer tenant y API key

Los endpoints de sistema de identity (`/v1/tenants`, `/v1/api-keys`) se protegen con el token de administrador que el compose fija en `LEDGERCORE_ADMIN_TOKEN=dev-admin-token`. Todo va a través del gateway (`http://localhost:8080`).

```bash
ADMIN='Authorization: Bearer dev-admin-token'

# 1) Crear el tenant
TENANT_ID=$(curl -s -X POST http://localhost:8080/v1/tenants \
  -H "$ADMIN" -H 'Content-Type: application/json' \
  -d '{"name":"Acme Payments"}' | jq -r .id)

# 2) Crear una API key (el secret se devuelve UNA sola vez)
KEY=$(curl -s -X POST http://localhost:8080/v1/api-keys \
  -H "$ADMIN" -H 'Content-Type: application/json' \
  -d "{\"tenant_id\":\"$TENANT_ID\",\"name\":\"local-dev\",\"environment\":\"sandbox\",\"scopes\":[\"ledger:read\",\"ledger:write\"]}")
API_KEY_ID=$(echo "$KEY" | jq -r .id)
API_KEY_SECRET=$(echo "$KEY" | jq -r .secret)

# 3) Canjear la key por un JWT (EdDSA, validado contra el JWKS de identity)
TOKEN=$(curl -s -X POST http://localhost:8080/v1/auth/token \
  -H 'Content-Type: application/json' \
  -d "{\"api_key_id\":\"$API_KEY_ID\",\"api_key_secret\":\"$API_KEY_SECRET\"}" | jq -r .access_token)
AUTH="Authorization: Bearer $TOKEN"
```

> Atajo de dev: el compose levanta los servicios con `LEDGERCORE_AUTH_DISABLED=true`, así que también puedes saltarte el JWT y mandar el header `X-Tenant-Id: $TENANT_ID` en su lugar. El flujo con token es el real; el header es solo comodidad local.

## Demo end-to-end: depósito de USD 100 (97 al cliente + 3 de fee)

Con el `$AUTH` del paso anterior:

```bash
# 4) Crear el ledger
LEDGER_ID=$(curl -s -X POST http://localhost:8080/v1/ledgers \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"name":"main","description":"Primary operating ledger"}' | jq -r .id)

# 5) Crear las tres cuentas
CASH=$(curl -s -X POST http://localhost:8080/v1/accounts \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"ledger_id\":\"$LEDGER_ID\",\"name\":\"assets:cash\",\"type\":\"asset\",\"normal_balance\":\"DEBIT\"}" | jq -r .id)

WALLET=$(curl -s -X POST http://localhost:8080/v1/accounts \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"ledger_id\":\"$LEDGER_ID\",\"name\":\"customer:cust_42:wallet\",\"type\":\"liability\",\"normal_balance\":\"CREDIT\"}" | jq -r .id)

FEES=$(curl -s -X POST http://localhost:8080/v1/accounts \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{\"ledger_id\":\"$LEDGER_ID\",\"name\":\"revenue:fees\",\"type\":\"revenue\",\"normal_balance\":\"CREDIT\"}" | jq -r .id)

# 6) Transacción balanceada: entra 100.00 USD, 97.00 al cliente y 3.00 de fee.
#    Montos SIEMPRE como enteros en unidades mínimas (USD exponente 2).
curl -s -X POST http://localhost:8080/v1/transactions \
  -H "$AUTH" -H 'Content-Type: application/json' \
  -d "{
    \"ledger_id\": \"$LEDGER_ID\",
    \"idempotency_key\": \"dep-2026-0001\",
    \"description\": \"Customer USD deposit\",
    \"postings\": [
      {\"account_id\": \"$CASH\",   \"direction\": \"DEBIT\",  \"amount\": {\"asset\": \"USD\", \"amount\": \"10000\"}},
      {\"account_id\": \"$WALLET\", \"direction\": \"CREDIT\", \"amount\": {\"asset\": \"USD\", \"amount\": \"9700\"}},
      {\"account_id\": \"$FEES\",   \"direction\": \"CREDIT\", \"amount\": {\"asset\": \"USD\", \"amount\": \"300\"}}
    ]
  }" | jq

# Repetir el mismo POST con la misma idempotency_key NO duplica el asiento:
# devuelve la transacción original (200 en lugar de 201).

# 7) Balance del wallet del cliente — se espera posted=9700, available=9700
curl -s http://localhost:8080/v1/accounts/$WALLET/balances -H "$AUTH" | jq

# 8) Trial balance del ledger — débitos == créditos por asset (balanced: true)
curl -s "http://localhost:8080/v1/trial-balance?ledger_id=$LEDGER_ID" -H "$AUTH" | jq
```

Si todo salió bien, el trial balance muestra `debits: 10000`, `credits: 10000` y `balanced: true` para USD, y el evento `ledger.transaction.posted` quedó publicado en NATS JetStream vía outbox.

Los contratos completos de las API (esquemas, ejemplos y códigos de error) viven en [`contracts/openapi/`](../contracts/).

## Salud de los servicios

Cada servicio expone `GET /healthz` (liveness) y `GET /readyz` (ping a Postgres) en su puerto directo, por ejemplo `http://localhost:8081/healthz`.

## CI

`.github/workflows/ci.yml` corre en cada push a `main` y en cada pull request:

- **go** — matriz por módulo (`libs/go` y los cuatro servicios): `go build`, `go vet`, `go test`.
- **web** — `pnpm install` + `pnpm build` de `apps/console` con Node 26.
- **compose-validate** — `docker compose config -q` sobre este stack.
