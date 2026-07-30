# SDKs oficiales de LedgerCore

| SDK | Paquete | Runtime | Estado |
| --- | --- | --- | --- |
| [`typescript/`](typescript/) | [`@ledgercore/sdk@0.1.0`](https://www.npmjs.com/package/@ledgercore/sdk) | Node 18+ / navegador, cero deps | **Publicado en npm** |
| [`php/`](php/) | [`ledgercore/sdk@v0.1.0`](https://packagist.org/packages/ledgercore/sdk) | PHP 8.1+, cURL nativo | **Publicado en Packagist** |

Ambos son clientes **escritos a mano** pero fieles a los contratos de `contracts/openapi/` (ledger, identity, webhooks). Comparten semántica:

- **Auth interna**: API key → JWT EdDSA de 15 min, cacheado, renovado con <60 s restantes, un reintento en 401.
- **Dinero**: montos siempre strings de unidades menores; `Money.fromDecimal` / `Money::fromDecimal` y `toDecimal` sin floats ni redondeo silencioso.
- **Idempotencia** de primera clase en `transactions.create` (UUID v4 autogenerado si no se pasa key).
- **Webhooks**: `verifySignature(payload, header, secret)` contra `X-LedgerCore-Signature` (`t=...,v1=...`), tiempo constante + ventana anti-replay de 5 min. Implementación de referencia: `services/webhooks/internal/signature`.
- **Errores tipados** con `status`, `code` y `requestId`.

Cada SDK trae un smoke E2E ejecutable (`examples/quickstart.{ts,php}`) que corre el flujo completo contra el stack local de `infra/compose`: signup sandbox → ledger → 3 cuentas → depósito 100/97/3 → statement → trial balance `balanced: true`.

## Instalación (desde el registry)

Ambos paquetes están **publicados** desde 2026-07-29 en la versión `0.1.0`. Instálalos desde su registry — **no** desde este monorepo.

**TypeScript / JavaScript — npm**

```sh
npm install @ledgercore/sdk
# o: pnpm add @ledgercore/sdk   /   yarn add @ledgercore/sdk
```

- Paquete: <https://www.npmjs.com/package/@ledgercore/sdk> (`@ledgercore/sdk@0.1.0`)
- Repositorio: <https://github.com/ASanchezT85/ledgercore-sdk-typescript>

**PHP — Composer / Packagist**

```sh
composer require ledgercore/sdk
```

- Paquete: <https://packagist.org/packages/ledgercore/sdk> (`ledgercore/sdk@v0.1.0`)
- Repositorio: <https://github.com/ASanchezT85/ledgercore-sdk-php>

Las copias en `sdks/typescript/` y `sdks/php/` de este monorepo son la **fuente de desarrollo**; los repos publicados se sincronizan desde aquí. Los consumidores siempre instalan desde npm/Packagist.

## Publicar una versión nueva

El versionado sigue semver y arranca en `0.1.0`.

**npm (`@ledgercore/sdk`)**

1. `cd sdks/typescript && npm run build && npm test`
2. Subir `version` en `package.json` y sincronizar el repo `ledgercore-sdk-typescript`.
3. `npm publish --access public` (scope `@ledgercore`, ya creado).

**Packagist (`ledgercore/sdk`)**

1. Sincronizar `sdks/php` hacia el repo público `ledgercore-sdk-php`.
2. Etiquetar la versión (`vX.Y.Z`) en ese repo; el webhook de Packagist auto-actualiza.

Al cambiar nombres de paquete o versión mayor, actualizar los snippets de instalación del quickstart de la console.
