# Catálogo de errores de la API

Todos los servicios de LedgerCore (identity, ledger-core, reconciliation,
webhooks) devuelven los errores 4xx/5xx con un único formato:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "explicación legible; nunca la parsees",
    "request_id": "9f1b2c3d4e5f60718293a4b5c6d7e8f9"
  }
}
```

- `code`: código estable en snake_case. Es la ÚNICA parte del error sobre la
  que un cliente debe ramificar.
- `message`: texto para humanos. Puede cambiar sin aviso.
- `request_id`: id de correlación de la petición. Siempre presente; es el
  mismo valor de la cabecera `X-Request-Id` (propagada si el cliente la
  envía, generada si no). Inclúyelo en cualquier reporte de soporte.

Ningún error expone detalles internos (SQL, stack traces, rutas de archivo).
Los errores inesperados se registran con detalle en los logs del servicio y
al cliente sólo le llega `internal`.

## Códigos

| Código | HTTP | Significado |
|---|---|---|
| `validation_failed` | 400 | Entrada malformada o semánticamente inválida (JSON inválido, campo desconocido, UUID mal formado, límite no numérico…). |
| `invalid_cursor` | 400 | El `?cursor=` de paginación no es un cursor emitido por la API. Usa siempre el `next_cursor` literal de la página anterior. |
| `unauthorized` | 401 | Credenciales ausentes o inválidas (token faltante/expirado, API key desconocida o revocada, tenant sin contexto, token de admin incorrecto). |
| `forbidden` | 403 | Autenticado pero sin permiso para la operación (reservado; hoy los scopes se validan de forma binaria en el gateway). |
| `not_found` | 404 | El recurso no existe dentro del tenant. |
| `conflict` | 409 | Conflicto de unicidad (slug/nombre duplicado) o de estado del ciclo de vida (postear una transacción revertida, reintentar una entrega no reintentable). |
| `idempotency_conflict` | 409 | Reservado: misma idempotency key con un payload distinto. Hoy la reutilización de una key devuelve la respuesta original con la cabecera `X-Idempotent-Replay: true`. |
| `unbalanced_transaction` | 422 | Los apuntes no cuadran por activo (débitos ≠ créditos) o hay overflow monetario. |
| `insufficient_funds` | 422 | El saldo disponible no cubre la operación (holds). |
| `rate_limited` | 429 | Se superó un límite de uso (p. ej. cupo diario de signups sandbox). |
| `internal` | 500 | Error inesperado; el detalle sólo está en los logs, correlacionable por `request_id`. |
| `service_unavailable` | 503 | Dependencia caída (base de datos) o funcionalidad no configurada (admin token ausente). Reintenta con backoff. |

Los códigos son un contrato: no se renombran ni se eliminan. Añadir un código
nuevo requiere actualizar este documento y los contratos OpenAPI.

## Paginación (contrato uniforme)

Todas las colecciones (`GET` de listado) aceptan:

- `?limit=` — tamaño de página. Default **50**, máximo **200** (valores
  mayores se recortan a 200; valores no numéricos o < 1 → `validation_failed`).
- `?cursor=` — cursor keyset opaco (base64url de `(created_at, id)`), tomado
  del `next_cursor` de la página anterior. Malformado → `invalid_cursor`.

Y responden:

```json
{ "data": [ ... ], "next_cursor": "b3BhY28..." }
```

`next_cursor` es `null` cuando NO hay más resultados: el servidor pide
`limit+1` filas internamente, así que la última página nunca obliga a pedir
una página extra vacía. El orden es estable: más recientes primero
(`created_at DESC, id DESC`; el statement de cuenta pagina por
`effective_at ASC, posting_id ASC`).

Implementación compartida: `libs/go/httpx` (`ParsePage`, `Window`,
`ListResponse`, constantes `Code*`).
