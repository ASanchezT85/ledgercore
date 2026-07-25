# ADR-007 — API pública REST, OpenAPI-first, SDKs generados

**Estado:** aceptada · **Fecha:** 2026-07-24

## Contexto
El producto ES el API. Los clientes objetivo (fintechs LatAm, PHP/Node/Python) valoran integración rápida sobre elegancia de protocolo. gRPC externo añade fricción de adopción.

## Decisión
- API pública **REST + JSON**, contratos **OpenAPI 3.1** en `contracts/openapi/` escritos ANTES que los handlers.
- Versionado por path (`/v1`): aditivos sí, breaking → `/v2` con 12 meses de convivencia y header `Sunset`.
- `Idempotency-Key` obligatoria en toda mutación; paginación keyset con cursor opaco; errores `{"error":{"code","message"}}` con códigos estables.
- SDKs **generados** desde OpenAPI (evaluar Speakeasy/Fern en fase 2): PHP y TypeScript primero, Python en fase 3.
- gRPC queda reservado como opción interna futura, nunca como requisito para clientes.

## Consecuencias
- (+) Un solo artefacto (el YAML) alimenta docs, SDKs, validación de drift en CI y el portal de desarrolladores.
- (−) Mantener el contrato primero exige disciplina: CI falla si el handler diverge del YAML (oasdiff, fase 2).
