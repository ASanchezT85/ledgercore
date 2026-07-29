import type { GuideContent } from "../../docs-ui";

const ERROR_SHAPE = `{
  "error": {
    "code": "validation_failed",
    "message": "explicación legible; nunca la parsees",
    "request_id": "9f1b2c3d4e5f60718293a4b5c6d7e8f9"
  }
}`;

const TS_ERRORS = `import { LedgerCore, LedgerCoreError, ERROR_CODES } from "@ledgercore/sdk";

try {
  await lc.transactions.create({ ledger_id, postings });
} catch (e) {
  if (e instanceof LedgerCoreError) {
    // Ramifica SOLO por e.code (estable); e.message puede cambiar sin aviso.
    switch (e.code) {
      case "unbalanced_transaction": /* corrige los montos */ break;
      case "rate_limited":           /* backoff + retry misma key */ break;
      default: console.error(e.status, e.code, e.requestId);
    }
  }
  // Credenciales rechazadas → AuthenticationError; red caída → ConnectionError
}`;

const PHP_ERRORS = `use LedgerCore\\ErrorCode;
use LedgerCore\\Exception\\LedgerCoreException;

try {
    $lc->transactions->create([...]);
} catch (LedgerCoreException $e) {
    // Ramifica SOLO por errorCode (estable); message puede cambiar sin aviso.
    match ($e->errorCode) {
        ErrorCode::UNBALANCED_TRANSACTION => corregirMontos(),
        ErrorCode::RATE_LIMITED           => reintentarConBackoff(),
        default => log_error($e->status, $e->errorCode, $e->requestId),
    };
}
// Credenciales rechazadas → AuthenticationException; red → ConnectionException`;

// Shared catalog rows (code + HTTP are language-neutral)
const CATALOG_ES: string[][] = [
  ["validation_failed", "400", "Entrada malformada o semánticamente inválida (JSON inválido, campo desconocido, UUID mal formado, límite no numérico…).", "Corrige el request; no reintentes sin cambiarlo."],
  ["invalid_cursor", "400", "El ?cursor= no es un cursor emitido por la API.", "Usa el next_cursor literal de la página anterior; si lo perdiste, reinicia desde la primera página."],
  ["unauthorized", "401", "Credenciales ausentes o inválidas: token faltante/expirado, API key desconocida o revocada.", "Renueva el token (los SDKs lo hacen solos y reintentan una vez). Si persiste, revisa la API key."],
  ["forbidden", "403", "Autenticado pero sin permiso para la operación (reservado; hoy los scopes se validan de forma binaria en el gateway).", "Revisa los scopes de tu llave."],
  ["not_found", "404", "El recurso no existe dentro de tu tenant.", "Verifica el id y que pertenezca al tenant/entorno correcto (sandbox vs live)."],
  ["conflict", "409", "Conflicto de unicidad (slug/nombre duplicado) o de estado del ciclo de vida (postear una transacción revertida, reintentar una entrega no reintentable).", "Lee el recurso actual y decide; no es un error transitorio."],
  ["idempotency_conflict", "409", "Reservado: misma idempotency key con payload distinto. Hoy la reutilización devuelve la respuesta original con X-Idempotent-Replay: true.", "Nunca reutilices una key con contenido distinto."],
  ["unbalanced_transaction", "422", "Los apuntes no cuadran por activo (débitos ≠ créditos) o hay overflow monetario.", "Corrige los montos hasta que débitos = créditos por asset."],
  ["insufficient_funds", "422", "El saldo disponible no cubre la operación (holds).", "Reduce el monto o libera holds; reintenta cuando haya saldo."],
  ["rate_limited", "429", "Se superó un límite de uso (p. ej. cupo diario de signups sandbox).", "Espera y reintenta con backoff; con la misma idempotency key el retry es seguro."],
  ["internal", "500", "Error inesperado; el detalle sólo está en los logs del servicio.", "Reintenta con backoff; si persiste, reporta el request_id a soporte."],
  ["service_unavailable", "503", "Dependencia caída (base de datos) o funcionalidad no configurada (admin token ausente).", "Reintenta con backoff."],
];

const CATALOG_EN: string[][] = [
  ["validation_failed", "400", "Malformed or semantically invalid input (bad JSON, unknown field, malformed UUID, non-numeric limit…).", "Fix the request; don't retry unchanged."],
  ["invalid_cursor", "400", "The ?cursor= is not a cursor issued by the API.", "Use the previous page's next_cursor verbatim; if lost, restart from the first page."],
  ["unauthorized", "401", "Missing or invalid credentials: missing/expired token, unknown or revoked API key.", "Refresh the token (the SDKs do it and retry once). If it persists, check the API key."],
  ["forbidden", "403", "Authenticated but not allowed (reserved; today scopes are validated binarily at the gateway).", "Review your key's scopes."],
  ["not_found", "404", "The resource does not exist within your tenant.", "Check the id and that it belongs to the right tenant/environment (sandbox vs live)."],
  ["conflict", "409", "Uniqueness conflict (duplicate slug/name) or lifecycle conflict (posting a reversed transaction, retrying a non-retryable delivery).", "Read the current resource and decide; it is not transient."],
  ["idempotency_conflict", "409", "Reserved: same idempotency key with a different payload. Today reuse returns the original response with X-Idempotent-Replay: true.", "Never reuse a key with different content."],
  ["unbalanced_transaction", "422", "Postings don't balance per asset (debits ≠ credits) or a monetary overflow occurred.", "Fix the amounts until debits = credits per asset."],
  ["insufficient_funds", "422", "Available balance does not cover the operation (holds).", "Lower the amount or release holds; retry once funded."],
  ["rate_limited", "429", "A usage limit was exceeded (e.g. daily sandbox signup quota).", "Wait and retry with backoff; with the same idempotency key the retry is safe."],
  ["internal", "500", "Unexpected error; details live only in the service logs.", "Retry with backoff; if it persists, report the request_id to support."],
  ["service_unavailable", "503", "Dependency down (database) or feature not configured (missing admin token).", "Retry with backoff."],
];

export const GUIDE: GuideContent = {
  es: {
    badge: "Guía · Errores",
    title: "Errores: un formato, un catálogo estable",
    subtitle:
      "Los cuatro servicios devuelven los errores 4xx/5xx con el mismo formato. El code en snake_case es la ÚNICA parte sobre la que un cliente debe ramificar: los códigos son un contrato — no se renombran ni se eliminan.",
    backLabel: "Docs",
    copy: "Copiar",
    copied: "Copiado",
    sections: [
      {
        title: "El formato único",
        blocks: [
          { kind: "resp", label: "Todo error 4xx/5xx", code: ERROR_SHAPE },
          {
            kind: "list",
            items: [
              "code: código estable en snake_case. Lo único sobre lo que debes ramificar.",
              "message: texto para humanos. Puede cambiar sin aviso — nunca lo parsees.",
              "request_id: id de correlación, siempre presente; es el mismo valor de la cabecera X-Request-Id. Inclúyelo en cualquier reporte de soporte.",
              "Ningún error expone detalles internos (SQL, stack traces, rutas): lo inesperado se registra en los logs y al cliente sólo le llega internal.",
            ],
          },
        ],
      },
      {
        title: "Catálogo completo",
        body: "Código → HTTP → cuándo ocurre → qué hacer:",
        blocks: [
          { kind: "table", head: ["Código", "HTTP", "Cuándo ocurre", "Qué hacer"], rows: CATALOG_ES },
        ],
      },
      {
        title: "Manejo en TypeScript",
        body: "Toda respuesta no-2xx lanza LedgerCoreError con status, code y requestId. El catálogo se exporta como ERROR_CODES (tipo ErrorCode).",
        blocks: [{ kind: "code", label: "TypeScript", code: TS_ERRORS }],
      },
      {
        title: "Manejo en PHP",
        body: "Toda respuesta no-2xx lanza LedgerCoreException con status, errorCode y requestId. El catálogo vive en LedgerCore\\ErrorCode (ErrorCode::ALL).",
        blocks: [{ kind: "code", label: "PHP", code: PHP_ERRORS }],
      },
    ],
  },
  en: {
    badge: "Guide · Errors",
    title: "Errors: one shape, one stable catalog",
    subtitle:
      "All four services return 4xx/5xx errors with the same shape. The snake_case code is the ONLY part a client should branch on: codes are a contract — never renamed, never removed.",
    backLabel: "Docs",
    copy: "Copy",
    copied: "Copied",
    sections: [
      {
        title: "The single shape",
        blocks: [
          { kind: "resp", label: "Every 4xx/5xx error", code: ERROR_SHAPE },
          {
            kind: "list",
            items: [
              "code: stable snake_case code. The only thing you should branch on.",
              "message: human-readable text. May change without notice — never parse it.",
              "request_id: correlation id, always present; same value as the X-Request-Id header. Include it in any support report.",
              "No error exposes internals (SQL, stack traces, file paths): unexpected failures are logged server-side and the client only sees internal.",
            ],
          },
        ],
      },
      {
        title: "Full catalog",
        body: "Code → HTTP → when it happens → what to do:",
        blocks: [
          { kind: "table", head: ["Code", "HTTP", "When it happens", "What to do"], rows: CATALOG_EN },
        ],
      },
      {
        title: "Handling in TypeScript",
        body: "Every non-2xx response throws LedgerCoreError with status, code and requestId. The catalog is exported as ERROR_CODES (type ErrorCode).",
        blocks: [{ kind: "code", label: "TypeScript", code: TS_ERRORS }],
      },
      {
        title: "Handling in PHP",
        body: "Every non-2xx response throws LedgerCoreException with status, errorCode and requestId. The catalog lives in LedgerCore\\ErrorCode (ErrorCode::ALL).",
        blocks: [{ kind: "code", label: "PHP", code: PHP_ERRORS }],
      },
    ],
  },
};
