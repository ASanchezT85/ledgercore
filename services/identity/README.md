# identity — Registro de tenants, API keys y emisión de tokens

Servicio de identidad de LedgerCore. Administra el registro de tenants (nivel
sistema), el ciclo de vida de las API keys y la emisión de JWT EdDSA de corta
vida, publicando sus claves públicas vía JWKS para que el resto de los
servicios validen tokens con `libs/go/ident.RequireAuth`.

- **Módulo:** `github.com/ASanchezT85/ledgercore/services/identity`
- **Puerto:** `8082`
- **Schema Postgres:** `identity` (el pool fija `search_path=identity`)
- **Arquitectura:** hexagonal — `cmd/server`, `internal/domain`, `internal/app`,
  `internal/adapters/{http,postgres}`

## Endpoints

| Método y ruta | Auth | Descripción |
| --- | --- | --- |
| `POST /v1/tenants` | Admin | Crea un tenant `{name, slug}`. |
| `GET /v1/tenants` | Admin | Lista todos los tenants. |
| `GET /v1/tenants/{id}` | Admin | Obtiene un tenant. |
| `POST /v1/api-keys` | Admin | Crea una API key `{tenant_id, environment, name}`. El secreto se devuelve **una sola vez**. |
| `DELETE /v1/api-keys/{id}` | Admin | Revoca una API key (idempotente). |
| `POST /v1/auth/token` | Pública | Intercambia `{api_key}` por un JWT EdDSA de 15 minutos. |
| `GET /.well-known/jwks.json` | Pública | JWKS con las claves públicas activas (OKP/Ed25519). |
| `GET /healthz` | Pública | Liveness. |
| `GET /readyz` | Pública | Readiness (ping a Postgres). |

**Auth admin (solo bootstrap de dev):** header `X-Admin-Token` comparado en
tiempo constante contra `LEDGERCORE_ADMIN_TOKEN`. Si la variable no está
definida, los endpoints admin responden `503 admin_disabled` (fail closed).

Los errores siguen el contrato de la plataforma:
`{"error":{"code":"...","message":"..."}}`.

## API keys

- Formato del secreto: `lk_<env>_<32 caracteres base62 de crypto/rand>`
  (ej. `lk_sandbox_Ab12...`). La generación usa *rejection sampling* para que
  no haya sesgo de módulo.
- Se persiste **solo** el hash SHA-256 (`secret_hash BYTEA`) y los primeros 12
  caracteres (`key_prefix`) para el lookup. El plaintext se muestra únicamente
  en la respuesta de creación.
- La verificación compara hashes con `crypto/subtle.ConstantTimeCompare`; si
  el prefix no matchea ninguna key se hace una comparación dummy para igualar
  el timing.
- SHA-256 (y no un KDF lento) es correcto aquí porque el secreto tiene ~190
  bits de entropía aleatoria; no es una contraseña humana.

## Emisión de tokens

`POST /v1/auth/token` busca por `key_prefix`, verifica el hash en tiempo
constante, exige key no revocada y tenant `active`, y emite un JWT EdDSA de
15 minutos con `kid` en el header. Respuesta:

```json
{ "access_token": "...", "token_type": "Bearer", "expires_in": 900 }
```

### Claims (contrato con libs/go/ident)

El middleware `libs/go/ident.RequireAuth` — que usan **todos** los servicios —
parsea los claims con los nombres `tenant_id`, `env` y `scopes`. Por eso el
emisor usa exactamente esos nombres (y no `tid`/`scope`): el contrato vigente
es el del validador compartido y está cubierto por un test de round-trip
(`TestIssueTokenRoundTripAgainstOwnJWKS`) que valida un token emitido por este
servicio contra su propio JWKS usando ese middleware.

```json
{
  "sub": "api_key:<uuid de la key>",
  "tenant_id": "<uuid del tenant>",
  "env": "sandbox | live",
  "scopes": ["ledger:read", "ledger:write"],
  "iss": "ledgercore-identity",
  "iat": 1753372800,
  "exp": 1753373700
}
```

## Clave de firma

Al arrancar, si no existe una `signing_key` activa se genera un par Ed25519,
se guarda en PEM (PKCS#8 / PKIX) y se loggea el `kid`. El JWKS publica todas
las claves activas, lo que permite rotación sin invalidar tokens en vuelo.

> **TODO(kms):** en producción la clave privada debe vivir en un KMS/HSM y el
> servicio debe firmar vía la API del KMS. Guardar el PEM en la base es solo
> un atajo de bootstrap para desarrollo.

## RLS: dónde aplica y dónde no (y por qué)

- **`tenants` — sin RLS.** Es la raíz del modelo multi-tenant: se lee para
  *establecer* el contexto de tenant, por lo que scoping por
  `app.tenant_id` sería circular. Es un registro de nivel sistema operado por
  la plataforma (la excepción prevista por la convención del monorepo).
- **`api_keys` — con RLS (dos políticas OR):** `tenant_isolation` (la política
  estándar por `app.tenant_id`) más `system_access`, que permite el acceso
  cuando NO hay contexto de tenant. Los flujos de sistema (bootstrap admin y
  el lookup por prefix del token — que ocurre *antes* de saber el tenant)
  corren sin contexto; cualquier conexión futura con contexto de tenant queda
  automáticamente aislada a sus propias keys. Se usa `NULLIF(..., '')` porque
  tras un `SET LOCAL` Postgres puede dejar el GUC como cadena vacía y el cast
  `''::uuid` reventaría la política estándar.
- **`signing_keys` — sin RLS.** Infraestructura de la plataforma; no pertenece
  a ningún tenant (la mitad pública se sirve al mundo por JWKS).

## Migraciones

Goose SQL embebidas (`embed.FS`) en
`internal/adapters/postgres/migrations/`. Corren al arrancar si
`LEDGERCORE_AUTO_MIGRATE=true`. La tabla de versiones vive en
`identity.goose_db_version`.

## Eventos

Este servicio **no publica eventos**: los topics de la plataforma
(`ledger.*`, `recon.*`) pertenecen a ledger-core y reconciliation, por lo que
no hay outbox ni conexión NATS aquí.

## Variables de entorno

Ver [.env.example](.env.example). Resumen: `LEDGERCORE_HTTP_ADDR` (default
`:8082`), `LEDGERCORE_DATABASE_URL` (requerida), `LEDGERCORE_ADMIN_TOKEN`,
`LEDGERCORE_AUTO_MIGRATE`, y `OTEL_EXPORTER_OTLP_ENDPOINT` (opcional, no-op si
falta).

## Desarrollo

```bash
cd services/identity
go mod tidy && go build ./... && go vet ./... && go test ./...
```

Los tests unitarios no necesitan base de datos. El test de integración
(`internal/adapters/postgres`) se salta salvo que definas:

```bash
LEDGERCORE_TEST_DATABASE_URL='postgres://ledgercore_app:ledgercore_dev@localhost:5432/ledgercore?sslmode=disable' go test ./internal/adapters/postgres/
```

### Docker

El contexto de build es la **raíz del repo** (copia `libs/go` y el servicio):

```bash
docker build -f services/identity/Dockerfile -t ledgercore/identity .
```

### Flujo de bootstrap en dev

```bash
# 1. Crear tenant
curl -s -X POST localhost:8082/v1/tenants \
  -H 'X-Admin-Token: dev-admin-token' \
  -d '{"name":"Acme Payments","slug":"acme-payments"}'

# 2. Crear API key (guardar el campo "secret": no vuelve a mostrarse)
curl -s -X POST localhost:8082/v1/api-keys \
  -H 'X-Admin-Token: dev-admin-token' \
  -d '{"tenant_id":"<uuid>","environment":"sandbox","name":"ci"}'

# 3. Canjear por un JWT
curl -s -X POST localhost:8082/v1/auth/token \
  -d '{"api_key":"lk_sandbox_..."}'
```
