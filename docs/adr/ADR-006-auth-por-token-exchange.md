# ADR-006 — Autenticación por token exchange (API key → JWT EdDSA)

**Estado:** aceptada · **Fecha:** 2026-07-24

## Contexto
Cuatro servicios deben autenticar cada request con el tenant correcto sin acoplarse todos a la base de datos de credenciales.

## Decisión
- `identity` es el único que conoce las API keys (almacena hash SHA-256 + prefix; el secreto se muestra una vez).
- El cliente intercambia `lk_<env>_…` por un **JWT EdDSA (Ed25519) de 15 minutos** con claims `{tid, env, scope}` (`POST /v1/auth/token`).
- Los demás servicios validan el JWT contra el **JWKS** público de identity (cacheado) — cero I/O a bases ajenas en el hot path.
- Dev: `LEDGERCORE_AUTH_DISABLED=true` toma el tenant de `X-Tenant-Id` (solo local).

## Consecuencias
- (+) Revocación de key efectiva en ≤15 min sin round-trip por request; enterprise podrá exigir TTL menor o introspección.
- (+) Rotación de claves de firma = publicar kid nuevo en el JWKS.
- (−) Las claves privadas viven en BD en dev; **KMS es requisito de fase 2 antes de pilotos**.
