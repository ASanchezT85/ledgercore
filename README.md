# LedgerCore

![build](https://img.shields.io/badge/build-pendiente-lightgrey) ![coverage](https://img.shields.io/badge/coverage-pendiente-lightgrey) ![go](https://img.shields.io/badge/go-1.26-00ADD8) ![license](https://img.shields.io/badge/license-propietaria-blue)

**LedgerCore** es una plataforma *Ledger as a Service* B2B: un ledger financiero de **doble entrada**, **multi-tenant** y **API-first** que permite a fintechs, marketplaces y plataformas de pagos registrar movimientos de dinero con garantías contables fuertes — sin construir su propio core contable.

## Por qué LedgerCore

- **Doble entrada estricta.** Toda transacción balancea débitos y créditos por asset; el desbalance es imposible por diseño.
- **Dinero en enteros.** Los montos se almacenan como enteros en unidades mínimas (centavos, satoshis) junto al código de asset y su exponente. Nunca floats.
- **Multi-tenant real.** Aislamiento por `tenant_id` con Row-Level Security de PostgreSQL en cada tabla de negocio.
- **Event-driven.** Cada hecho contable se publica en NATS JetStream mediante patrón outbox transaccional: si la transacción no se confirmó, el evento no existe.
- **Contratos primero.** Las API se definen en OpenAPI 3.1 versionado; los SDK se generan desde los contratos.

## Mapa del monorepo

```
ledgercore/
├── services/          # Microservicios Go (un módulo por servicio)
│   ├── ledger-core/   # El ledger: ledgers, accounts, transactions, holds (8081)
│   ├── identity/      # Tenants, API keys, emisión de JWT, JWKS (8082)
│   ├── reconciliation/# Importación de fuentes externas y conciliación (8083)
│   └── webhooks/      # Suscripciones y entrega de webhooks firmados (8084)
├── libs/
│   └── go/            # Librería compartida: money, ident, events, pgxutil, httpx, obs
├── apps/
│   └── console/       # Consola web para tenants (Next.js, 3000)
├── contracts/         # OpenAPI 3.1 + JSON Schemas de eventos (fuente de verdad)
├── infra/             # docker-compose, Traefik, observabilidad, IaC
└── docs/              # Documentación de arquitectura y ADRs
```

### Puertos (entorno dev)

| Componente       | Puerto |
|------------------|--------|
| Gateway (Traefik)| 8080   |
| ledger-core      | 8081   |
| identity         | 8082   |
| reconciliation   | 8083   |
| webhooks         | 8084   |
| console          | 3000   |
| PostgreSQL 17    | 5432   |
| NATS (monitor)   | 4222 (8222) |
| Grafana          | 3100   |

## Quickstart

Requisitos: Docker + Docker Compose, Go 1.26+, Node 26 + pnpm (solo para la consola).

```bash
git clone https://github.com/ledgercore/ledgercore.git
cd ledgercore

# Levanta Postgres, NATS, Traefik y los cuatro servicios
docker compose -f infra/docker-compose.yml up -d

# O bien, en modo desarrollo local (servicios fuera de Docker)
make dev
```

Con el stack arriba, el primer asiento se registra así:

```bash
# 1. Token de sandbox (identity)
TOKEN=$(curl -s -X POST http://localhost:8080/identity/v1/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"api_key_id":"...","api_key_secret":"..."}' | jq -r .access_token)

# 2. Depósito de USD 100: 97 al cliente + 3 de fee
curl -s -X POST http://localhost:8080/ledger/v1/transactions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "ledger_id": "1f0a...",
    "idempotency_key": "dep-2026-0001",
    "description": "Customer USD deposit",
    "postings": [
      {"account_id": "acc_cash",     "direction": "DEBIT",  "amount": {"asset": "USD", "amount": "10000"}},
      {"account_id": "acc_customer", "direction": "CREDIT", "amount": {"asset": "USD", "amount": "9700"}},
      {"account_id": "acc_fees",     "direction": "CREDIT", "amount": {"asset": "USD", "amount": "300"}}
    ]
  }'
```

## Convenciones clave

- **Go 1.26**, `net/http` + `ServeMux` con method patterns. Sin frameworks HTTP.
- **PostgreSQL 17**, una instancia dev compartida, **un schema por servicio** (`ledger`, `identity`, `recon`, `webhooks`). Migraciones goose embebidas que corren al arrancar con `LEDGERCORE_AUTO_MIGRATE=true`.
- **Auth**: JWT EdDSA emitido por `identity` y validado contra su JWKS. En dev, `LEDGERCORE_AUTH_DISABLED=true` permite operar con el header `X-Tenant-Id`.
- **Observabilidad**: `log/slog` JSON en todos los servicios; OpenTelemetry vía variables estándar `OTEL_*` (no-op si no están definidas).
- **Idioma**: código y comentarios en inglés; documentación en español.

## Contratos y SDKs

La fuente de verdad de las API vive en [`contracts/`](contracts/README.md). `v1` está congelado: solo se admiten cambios aditivos; un cambio breaking implica `v2`.

## Licencia

Propietaria. © LedgerCore.
