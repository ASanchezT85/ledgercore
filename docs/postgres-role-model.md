# Modelo de roles de PostgreSQL (LC-014 / LC-002)

Este documento describe la separación de roles de PostgreSQL de LedgerCore: el
modelo de tres niveles definido en `infra/postgres/init/01-init.sql`, por qué
cierra los hallazgos **LC-014** (radio de impacto total) y **LC-002** (RLS
evitable), y la matriz de privilegios.

## Motivación

La auditoría encontró que **todos los servicios compartían un único rol**
(`ledgercore_app`) que además era **dueño de todos los esquemas**. Dos
consecuencias graves:

- **LC-014 — radio de impacto total.** Cualquier servicio comprometido podía
  leer y escribir los esquemas de los demás (ledger, identity, recon,
  webhooks) porque el rol compartido tenía acceso a todo.
- **LC-002 — RLS evitable.** Como el rol era **dueño** de las tablas, las
  políticas de Row-Level Security no se le aplicaban salvo que cada tabla
  declarara `FORCE ROW LEVEL SECURITY`. Un solo `ALTER TABLE` olvidado dejaba
  una tabla sin aislamiento de tenant.

## El modelo de tres niveles

```
                         ┌──────────────────────┐
        DDL (migraciones)│  ledgercore_migrator  │  dueño de los 4 esquemas
        NUNCA en runtime └───────────┬──────────┘  y de todas las tablas
                                     │ crea tablas → ALTER DEFAULT PRIVILEGES
                 ┌───────────────────┼───────────────────┐
                 ▼                   ▼                   ▼
   ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐
   │ ledgercore_ledger_rt│ │ledgercore_identity_rt│ …  (uno por servicio)
   │  DML solo en `ledger`│ │ DML solo en `identity`│
   │  NOBYPASSRLS, no DDL │ │ NOBYPASSRLS, no DDL  │
   └───────────────────┘  └───────────────────┘
                 ▲
                 │ EXECUTE de funciones SECURITY DEFINER
   ┌───────────────────────────────────────────────┐
   │ ledgercore_maint  (NOLOGIN)                     │
   │ dueño de las funciones de purga/mantenimiento   │
   │ credenciales NO disponibles en runtime          │
   └───────────────────────────────────────────────┘
```

### 1. `ledgercore_migrator` — dueño / migrador

- `LOGIN`, `NOSUPERUSER`, `NOBYPASSRLS`.
- Es **dueño de los cuatro esquemas y de todas sus tablas**.
- Ejecuta **solo DDL** (las migraciones goose de cada servicio). **Nunca** se
  usa como identidad de un servicio en runtime.
- `ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator ...` hace que cada
  tabla/secuencia que cree conceda automáticamente el DML correcto al rol
  runtime de ese esquema (no hay que re-conceder tras cada migración).
- Es miembro de `ledgercore_maint`, para poder reasignar a ese rol la
  propiedad de las funciones `SECURITY DEFINER`.

### 2. `ledgercore_<servicio>_rt` — runtime (uno por servicio)

`ledgercore_ledger_rt`, `ledgercore_identity_rt`, `ledgercore_recon_rt`,
`ledgercore_webhooks_rt`:

- `LOGIN`, `NOSUPERUSER`, **`NOBYPASSRLS`**, `NOCREATEDB`, `NOCREATEROLE`.
- Es la **única** identidad con la que se conecta el proceso del servicio.
- Tiene `USAGE` + **DML** (`SELECT/INSERT/UPDATE/DELETE`) **solo sobre su
  propio esquema**. **No** tiene `CREATE` en ningún esquema → no puede correr
  DDL, ni siquiera en el suyo.
- No tiene ningún privilegio sobre esquemas ajenos; además se hace un `REVOKE`
  explícito cruzado como defensa en profundidad.
- Como **no es dueño** de las tablas y es `NOBYPASSRLS`, las políticas RLS de
  tenant se le aplican **siempre**, incluso sin `FORCE`. (Las migraciones
  igual ponen `FORCE ROW LEVEL SECURITY`, cubriendo también a `migrator` y
  `maint`.) Esto es lo que cierra **LC-002**.

### 3. `ledgercore_maint` — mantenimiento

- **`NOLOGIN`**, sin contraseña: no puede autenticarse ningún servicio como él.
- Es **dueño de las funciones `SECURITY DEFINER`** de purga/mantenimiento que
  crean las migraciones de ledger-core (p. ej. `purge_expired_sandbox_tenant`).
  Al ser dueño + `SECURITY DEFINER`, dentro de esas funciones
  `current_user = 'ledgercore_maint'`, que es la condición que habilita el
  bypass controlado de los triggers append-only.
- Alcanzable solo de dos formas: (a) como *definer* de sus funciones, y (b) vía
  `SET ROLE` por un miembro (el migrador, o un superusuario humano) para
  reparación controlada. Un rol runtime **no** puede `SET ROLE` a él.
- Privilegios: `USAGE, CREATE` sobre los esquemas (el `CREATE` es necesario
  para que una función pueda ser de su propiedad) y `SELECT, DELETE` sobre las
  tablas (solo lee y borra; nunca `INSERT/UPDATE`).

## Matriz de roles y privilegios

| Rol | LOGIN | SUPERUSER | BYPASSRLS | Dueño de esquemas | DDL | DML propio esquema | Acceso a otros esquemas | Uso |
|-----|:-----:|:---------:|:---------:|:-----------------:|:---:|:------------------:|:-----------------------:|-----|
| `ledgercore_migrator` | sí | no | no | **sí (los 4)** | **sí** | sí (dueño) | sí (dueño de todos) | migraciones (paso `migrate`) |
| `ledgercore_ledger_rt` | sí | no | no | no | **no** | `SELECT/INSERT/UPDATE/DELETE` en `ledger` | **ninguno** | runtime de ledger-core |
| `ledgercore_identity_rt` | sí | no | no | no | **no** | idem en `identity` | **ninguno** | runtime de identity |
| `ledgercore_recon_rt` | sí | no | no | no | **no** | idem en `recon` | **ninguno** | runtime de reconciliation |
| `ledgercore_webhooks_rt` | sí | no | no | no | **no** | idem en `webhooks` | **ninguno** | runtime de webhooks |
| `ledgercore_maint` | **no** | no | no | no | vía funciones | `SELECT/DELETE` en los 4 | `SELECT/DELETE` en los 4 | purga/mantenimiento (SECURITY DEFINER) |

### DSN por servicio (dev — `infra/compose/docker-compose.yml`)

| Servicio | Rol de runtime | search_path | `AUTO_MIGRATE` |
|----------|----------------|-------------|:--------------:|
| ledger-core | `ledgercore_ledger_rt` | `ledger` | `false` |
| identity | `ledgercore_identity_rt` | `identity` | `false` |
| reconciliation | `ledgercore_recon_rt` | `recon` | `false` |
| webhooks | `ledgercore_webhooks_rt` | `webhooks` | `false` |

Las contraseñas del stack local son **solo de desarrollo** y las siembra
`01-init.sql`. En la sandbox pública el superusuario se sobrescribe por
entorno; los datos son desechables.

## Cómo corren las migraciones (separación DDL/runtime)

Los binarios de servicio ya no migran al arrancar (`LEDGERCORE_AUTO_MIGRATE=false`).
En su lugar, el compose trae un **one-shot `migrate`** que:

1. corre las migraciones goose de los cuatro servicios **como
   `ledgercore_migrator`** (el único con DDL), cada una con su `search_path`;
2. ejecuta `infra/postgres/migrate/grants.sql`, que reasigna a
   `ledgercore_maint` la propiedad de toda función `SECURITY DEFINER` de cada
   esquema y concede `EXECUTE` al rol runtime correspondiente.

Los servicios declaran `depends_on: migrate: condition: service_completed_successfully`,
así arrancan solo después de que el esquema esté migrado, y lo hacen con un rol
sin DDL.

## Verificación

Localmente (Docker) y en CI se comprueba que:

- las migraciones aplican como `migrator`;
- un rol runtime **puede** hacer DML de su esquema pero **no** DDL;
- un rol runtime **no** puede leer un esquema ajeno;
- un rol runtime **no** puede `SET ROLE ledgercore_maint`;
- todos los `*_rt` son `NOSUPERUSER` + `NOBYPASSRLS`.

El job `postgres role separation (init SQL)` de `.github/workflows/ci.yml`
ejecuta estas aserciones en cada PR; el job `rls contract` prueba el
aislamiento de tenant con las políticas RLS reales.

## Contrato de coordinación con los servicios

Los servicios (migraciones en `services/*`) asumen exactamente estos nombres:

- runtime sin DDL → las migraciones **no** crean tablas asumiendo ser dueñas en
  runtime; el dueño es `ledgercore_migrator`;
- `FORCE ROW LEVEL SECURITY` en toda tabla con `tenant_id` (defensa extra);
- `ledgercore_maint` como dueño de sus funciones `SECURITY DEFINER` de purga
  (la reasignación de propiedad y el `GRANT EXECUTE` los hace infra en
  `grants.sql`, no la migración).
