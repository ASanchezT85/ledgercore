# LedgerCore Console

Consola web de LedgerCore: el dashboard que usan los equipos de finanzas y los
desarrolladores de los clientes para operar el ledger — balances custodiados,
transacciones de doble partida, conciliación contra fuentes externas y
herramientas de integración (API keys, webhooks).

Construida con **Next.js (App Router) + TypeScript strict + Tailwind CSS v4**,
tema oscuro por defecto y componentes UI propios (sin librerías de componentes
externas).

## Requisitos

- Node.js 26+
- pnpm 11+

## Arranque rápido

```bash
cd apps/console
pnpm install
pnpm dev        # http://localhost:3000
```

Scripts disponibles:

| Script       | Descripción                                   |
| ------------ | --------------------------------------------- |
| `pnpm dev`   | Servidor de desarrollo en el puerto 3000      |
| `pnpm build` | Build de producción (salida `standalone`)     |
| `pnpm start` | Sirve el build de producción                  |
| `pnpm lint`  | Chequeo de tipos estricto (`tsc --noEmit`)    |

## Configuración

Copia `.env.example` a `.env.local`:

```bash
NEXT_PUBLIC_API_URL=http://localhost:8080
```

`NEXT_PUBLIC_API_URL` apunta al gateway (Traefik) que enruta hacia los
servicios Go (`ledger-core`, `identity`, `reconciliation`, `webhooks`).

### Modo demo

Los servicios Go se construyen en paralelo, así que la consola **nunca asume
que el API está disponible**. La capa de datos (`lib/api.ts`) intenta cada
fetch contra `NEXT_PUBLIC_API_URL` y, si la variable no está definida, el
request falla o supera el timeout, responde con **datos de demostración**
realistas y consistentes. Toda página que esté renderizando datos demo lo
marca visualmente con el badge «datos de demostración».

## Páginas

| Ruta              | Contenido                                                                 |
| ----------------- | ------------------------------------------------------------------------- |
| `/login`          | Pantalla stub (SSO próximamente) con acceso en modo demo                  |
| `/`               | Dashboard: balance custodiado, transacciones hoy, discrepancias, salud de conciliación |
| `/ledgers`        | Ledgers y árbol de cuentas (por `path`) con balances por asset            |
| `/transactions`   | Tabla con filtros (estado, asset) y drawer con los postings débito/crédito |
| `/reconciliation` | Corridas recientes y discrepancias (`missing_internal`, `amount_mismatch`, …) |
| `/developers`     | API keys, webhook subscriptions y snippet curl de `POST /v1/transactions` |

## Manejo de dinero

Los montos viajan y se almacenan como **enteros en unidades mínimas** más un
código de asset y su exponente (registro de assets). El helper
`formatMoney(units, asset, exponent)` en `lib/format.ts` formatea con BigInt y
numerales tabulares — nunca floats.

## Estructura

```
apps/console/
├── app/
│   ├── (console)/          # layout con sidebar + páginas autenticadas
│   │   ├── page.tsx        # dashboard
│   │   ├── ledgers/
│   │   ├── transactions/
│   │   ├── reconciliation/
│   │   └── developers/
│   ├── login/
│   ├── globals.css         # tokens de diseño (Tailwind v4 @theme)
│   └── layout.tsx
├── components/
│   ├── ui/                 # Button, Card, Badge, Table, StatTile, Sidebar
│   ├── logo.tsx            # logomark SVG inline
│   └── demo-badge.tsx
└── lib/
    ├── api.ts              # fetchers con fallback a datos demo
    ├── format.ts           # formatMoney / fechas deterministas
    ├── mock-data.ts
    └── types.ts
```

## Docker

El `Dockerfile` es multi-stage (`node:26-alpine`) y usa la salida
`standalone` de Next.js. El contexto de build es la **raíz del repo**:

```bash
docker build -f apps/console/Dockerfile -t ledgercore/console .
docker run -p 3000:3000 -e NEXT_PUBLIC_API_URL=http://gateway:8080 ledgercore/console
```

> Nota: `NEXT_PUBLIC_*` se congela en build; para cambiar el gateway en
> producción hay que reconstruir la imagen o resolverlo vía el gateway mismo.
