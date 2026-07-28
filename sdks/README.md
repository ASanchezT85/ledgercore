# SDKs oficiales de LedgerCore

| SDK | Paquete | Runtime | Estado |
| --- | --- | --- | --- |
| [`typescript/`](typescript/) | `@ledgercore/sdk` | Node 18+ / navegador, cero deps | Listo para publicar en npm |
| [`php/`](php/) | `ledgercore/sdk` | PHP 8.1+, cURL nativo | Listo para publicar en Packagist |

Ambos son clientes escritos a mano pero fieles a los contratos de `contracts/openapi/` (ledger, identity, webhooks). Comparten semántica:

- **Auth interna**: API key → JWT EdDSA de 15 min, cacheado, renovado con <60 s restantes, un reintento en 401.
- **Dinero**: montos siempre strings de unidades menores; `Money.fromDecimal` / `Money::fromDecimal` y `toDecimal` sin floats ni redondeo silencioso.
- **Idempotencia** de primera clase en `transactions.create` (UUID v4 autogenerado si no se pasa key).
- **Webhooks**: `verifySignature(payload, header, secret)` contra `X-LedgerCore-Signature` (`t=...,v1=...`), tiempo constante + ventana anti-replay de 5 min. Implementación de referencia: `services/webhooks/internal/signature`.
- **Errores tipados** con `status`, `code` y `requestId`.

Cada SDK trae un smoke E2E ejecutable (`examples/quickstart.{ts,php}`) que corre el flujo completo contra el stack local de `infra/compose`: signup sandbox → ledger → 3 cuentas → depósito 100/97/3 → statement → trial balance `balanced: true`.

## Publicación (pendiente)

Ninguno de los dos paquetes está publicado todavía. Cuando toque:

**npm (`@ledgercore/sdk`)**

1. Crear la organización `ledgercore` en npmjs.com (o ajustar el scope).
2. `cd sdks/typescript && npm run build && npm test`
3. Revisar `version` en `package.json` (semver) y `npm publish --access public`.

**Packagist (`ledgercore/sdk`)**

1. Extraer/replicar `sdks/php` en un repo público propio (Packagist indexa repos git, no subdirectorios de monorepo) o usar un split automático (p. ej. `git subtree split`/GitHub Action) hacia `ledgercore/ledgercore-php`.
2. Etiquetar `v0.1.0` en ese repo.
3. Registrar el repo en packagist.org y activar el webhook de auto-update.

Al publicar, actualizar los snippets de instalación del quickstart de la console si cambian los nombres de paquete.
