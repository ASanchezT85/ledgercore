# Contratos de LedgerCore

Este directorio es la **fuente de verdad** de todas las interfaces públicas de la plataforma: las API HTTP (OpenAPI 3.1) y los eventos asincrónicos (JSON Schema draft 2020-12). El código de los servicios implementa estos contratos; nunca al revés.

## Estructura

```
contracts/
├── openapi/
│   ├── ledger.v1.yaml           # API del ledger (ledgers, accounts, transactions, holds, trial balance)
│   ├── identity.v1.yaml         # Tenants, API keys, emisión de tokens, JWKS
│   ├── reconciliation.v1.yaml   # Fuentes externas, imports CSV, runs, reportes y discrepancias (prefijo /v1/reconciliation)
│   └── webhooks.v1.yaml         # Suscripciones y deliveries de webhooks (prefijos /v1/webhook-subscriptions y /v1/webhook-deliveries)
└── events/
    ├── envelope.schema.json                       # Sobre común de todos los eventos
    ├── ledger.transaction.posted.schema.json      # Un schema por topic…
    ├── ledger.transaction.reversed.schema.json
    ├── ledger.hold.created.schema.json
    ├── ledger.hold.captured.schema.json
    ├── ledger.hold.released.schema.json
    └── recon.discrepancy.detected.schema.json
```

## Reglas de versionado

1. **`v1` está congelado.** Una vez publicada, una versión de contrato no cambia de forma incompatible. Los clientes deben poder actualizar su SDK sin romper.
2. **Solo cambios aditivos dentro de una versión.** Se permite, sin subir versión:
   - agregar endpoints nuevos;
   - agregar campos **opcionales** en requests;
   - agregar campos en responses (los clientes deben tolerar campos desconocidos);
   - agregar valores nuevos a enums **solo** cuando el contrato lo declare abierto.
3. **Un cambio breaking implica versión nueva (`v2`).** Son breaking, entre otros: eliminar o renombrar campos, volver requerido un campo opcional, cambiar tipos o semántica de un campo, cambiar códigos de error documentados. La `v1` y la `v2` conviven durante el período de deprecación anunciado.
4. **Los eventos siguen la misma regla** con el campo `version` del envelope: cambios aditivos mantienen `version: 1`; un cambio estructural incrementa la versión y los consumidores deben manejar ambas durante la transición.

## Convenciones transversales

- **Dinero**: `{ "asset": "USD", "amount": "10000" }` — enteros en unidades mínimas, codificados como string para no perder precisión int64 en JSON. El exponente de cada asset vive en el registro de assets del ledger.
- **Direcciones**: `DEBIT` / `CREDIT`. Toda transacción balancea débitos y créditos por asset.
- **Errores**: siempre `{ "error": { "code": "...", "message": "..." } }`. El `code` es estable y parseable; el `message` es solo para humanos.
- **Paginación**: keyset con cursores opacos (`cursor` / `next_cursor`) en las colecciones grandes (ledger y webhook-deliveries). Las colecciones chicas (sources, subscriptions, tenants, discrepancies) hoy no paginan.
- **Autenticación**: JWT EdDSA emitido por `identity` (esquema `bearerAuth`). Claims del token: `tenant_id` (UUID), `env` (`sandbox`|`live`), `scopes` (hoy siempre `["ledger:read","ledger:write"]`), más los registrados (`sub` = id de la API key, `iat`, `exp`; TTL 15 min). Los endpoints de administración de identity usan `X-Admin-Token`, no bearer.
- **Idempotencia**: `idempotency_key` por tenant en transactions y holds; un replay responde 200 con el header `X-Idempotent-Replay: true`. En transactions también se acepta el header `Idempotency-Key`.
- **Stubs 501**: los endpoints declarados pero aún no implementados (fase 1.5) responden `501 not_implemented` y están marcados en cada spec (`/v1/statements`, `/v1/provider-positions`; el `as_of` del trial balance se acepta pero se ignora).
- **Health**: cada servicio expone `GET /healthz` y `GET /readyz` fuera de auth; son operacionales y quedan fuera de los contratos.

## Generación de SDKs

Los SDK oficiales (TypeScript, Go, etc.) **se generan desde estos OpenAPI**; no se escriben a mano. Cualquier cambio de contrato pasa por PR sobre este directorio, se revisa aquí, y recién después se regeneran clientes y stubs de servidor. Si el código y el contrato divergen, el contrato manda.
