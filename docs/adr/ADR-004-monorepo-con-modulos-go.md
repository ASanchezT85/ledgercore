# ADR-004 — Monorepo con módulos Go independientes

**Estado:** aceptada · **Fecha:** 2026-07-24

## Contexto
Elegimos 4-6 servicios bien delimitados (ni monolito ni microservicios finos). Hay que decidir cómo se organizan los repositorios y los módulos.

## Decisión
**Un monorepo** (`ledgercore/`) con:
- Un módulo Go por servicio (`services/<x>/go.mod`) + un módulo compartido `libs/go`, enlazados por `replace` relativos y `go.work` en la raíz.
- Apps TypeScript bajo `apps/` (pnpm).
- Contratos (`contracts/`) e infraestructura (`infra/`) versionados junto al código.
- CI por matriz de módulos: cada servicio compila, se testea y se dockeriza de forma independiente.

## Consecuencias
- (+) Un PR puede cambiar contrato + servicio + consola de forma atómica — crítico con equipo pequeño.
- (+) Cada servicio conserva build/deploy independiente (contexto de build = raíz, Dockerfile propio).
- (−) El monorepo exige disciplina de ownership por directorio; se refuerza con CODEOWNERS cuando crezca el equipo.
