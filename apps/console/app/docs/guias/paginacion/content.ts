import type { GuideContent } from "../../docs-ui";

const CURL_PAGE = `curl -s "$API/v1/transactions?ledger_id=$LEDGER_ID&limit=100" \\
  -H "Authorization: Bearer $TOKEN" | jq '{count: (.data|length), next_cursor}'`;

const RESP_PAGE = `{ "data": [ …100 transacciones… ], "next_cursor": "b3BhY28uY3Vyc29y..." }

// última página:
{ "data": [ …23 transacciones… ], "next_cursor": null }`;

const CURL_NEXT = `curl -s "$API/v1/transactions?ledger_id=$LEDGER_ID&limit=100&cursor=b3BhY28uY3Vyc29y..." \\
  -H "Authorization: Bearer $TOKEN" | jq`;

const TS_PAGE = `// Página a página
let cursor: string | undefined;
do {
  const page = await lc.transactions.list({ ledgerId, limit: 100, cursor });
  for (const txn of page.data) { /* ... */ }
  cursor = page.next_cursor ?? undefined;
} while (cursor);

// O autopaginación: listAll() en todas las colecciones
for await (const txn of lc.transactions.listAll({ ledgerId })) { /* ... */ }`;

const PHP_PAGE = `// Página a página
$cursor = null;
do {
    $page = $lc->transactions->list(limit: 100, cursor: $cursor);
    foreach ($page['data'] as $txn) { /* ... */ }
    $cursor = $page['next_cursor'];
} while ($cursor !== null);

// O autopaginación: generadores listAll() en todas las colecciones
foreach ($lc->transactions->listAll($ledgerId) as $txn) { /* ... */ }`;

export const GUIDE: GuideContent = {
  es: {
    badge: "Guía · Paginación",
    title: "Paginación: limit, cursor y next_cursor",
    subtitle:
      "Todas las colecciones de la API paginan igual: cursor keyset opaco, default 50 por página, máximo 200, y next_cursor null en la última página. Los SDKs añaden autopaginación con listAll().",
    backLabel: "Docs",
    copy: "Copiar",
    copied: "Copiado",
    sections: [
      {
        title: "El contrato uniforme",
        body: "Cada GET de listado acepta dos query params y responde siempre la misma envolvente { data, next_cursor }.",
        blocks: [
          {
            kind: "table",
            head: ["Parámetro", "Semántica"],
            rows: [
              [
                "?limit=",
                "Tamaño de página. Default 50, máximo 200 (valores mayores se recortan a 200; no numéricos o < 1 → 400 validation_failed).",
              ],
              [
                "?cursor=",
                "Cursor keyset opaco (base64url de (created_at, id)), tomado literal del next_cursor de la página anterior. Malformado → 400 invalid_cursor.",
              ],
            ],
          },
          { kind: "code", label: "GET con limit", code: CURL_PAGE },
          { kind: "resp", label: "Envolvente de respuesta", code: RESP_PAGE },
        ],
      },
      {
        title: "Recorre con next_cursor",
        body: "Pasa el next_cursor literal de la página anterior. next_cursor es null cuando NO hay más resultados: el servidor pide limit+1 filas internamente, así que la última página nunca te obliga a pedir una página extra vacía.",
        blocks: [{ kind: "code", label: "Página siguiente", code: CURL_NEXT }],
      },
      {
        title: "Orden estable",
        body: "Más recientes primero: created_at DESC, id DESC. La excepción es el statement de cuenta, que pagina cronológicamente por effective_at ASC, posting_id ASC. Al ser cursor keyset (no offset), insertar filas nuevas no desplaza ni duplica resultados mientras recorres.",
      },
      {
        title: "invalid_cursor: no fabriques cursores",
        body: "El cursor es opaco: no lo construyas, no lo edites, no lo persistas entre despliegues como si fuera un formato estable. Si la API no lo reconoce responde 400 invalid_cursor — reinicia el recorrido desde la primera página.",
      },
      {
        title: "Autopaginación en los SDKs",
        body: "Toda colección tiene list({ limit, cursor }) y listAll(): un iterador (async generator en TS, generator en PHP) que pide páginas bajo demanda hasta agotar next_cursor.",
        blocks: [
          { kind: "code", label: "TypeScript", code: TS_PAGE },
          { kind: "code", label: "PHP", code: PHP_PAGE },
        ],
      },
    ],
  },
  en: {
    badge: "Guide · Pagination",
    title: "Pagination: limit, cursor and next_cursor",
    subtitle:
      "Every collection in the API paginates the same way: opaque keyset cursor, default 50 per page, max 200, and next_cursor null on the last page. The SDKs add auto-pagination via listAll().",
    backLabel: "Docs",
    copy: "Copy",
    copied: "Copied",
    sections: [
      {
        title: "The uniform contract",
        body: "Every list GET accepts two query params and always answers the same envelope { data, next_cursor }.",
        blocks: [
          {
            kind: "table",
            head: ["Parameter", "Semantics"],
            rows: [
              [
                "?limit=",
                "Page size. Default 50, max 200 (larger values are clamped to 200; non-numeric or < 1 → 400 validation_failed).",
              ],
              [
                "?cursor=",
                "Opaque keyset cursor (base64url of (created_at, id)), taken verbatim from the previous page's next_cursor. Malformed → 400 invalid_cursor.",
              ],
            ],
          },
          { kind: "code", label: "GET with limit", code: CURL_PAGE },
          { kind: "resp", label: "Response envelope", code: RESP_PAGE },
        ],
      },
      {
        title: "Walk with next_cursor",
        body: "Pass the previous page's next_cursor verbatim. next_cursor is null when there are NO more results: the server fetches limit+1 rows internally, so the last page never forces an extra empty request.",
        blocks: [{ kind: "code", label: "Next page", code: CURL_NEXT }],
      },
      {
        title: "Stable ordering",
        body: "Newest first: created_at DESC, id DESC. The exception is the account statement, which paginates chronologically by effective_at ASC, posting_id ASC. Being keyset (not offset) cursors, new inserts never shift or duplicate results mid-walk.",
      },
      {
        title: "invalid_cursor: never fabricate cursors",
        body: "The cursor is opaque: don't build it, don't edit it, don't persist it across deployments as if it were a stable format. If the API doesn't recognize it, it answers 400 invalid_cursor — restart the walk from the first page.",
      },
      {
        title: "Auto-pagination in the SDKs",
        body: "Every collection has list({ limit, cursor }) and listAll(): an iterator (async generator in TS, generator in PHP) that fetches pages on demand until next_cursor runs out.",
        blocks: [
          { kind: "code", label: "TypeScript", code: TS_PAGE },
          { kind: "code", label: "PHP", code: PHP_PAGE },
        ],
      },
    ],
  },
};
