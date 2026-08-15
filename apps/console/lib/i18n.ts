export type Lang = "es" | "en";

export const LANG_STORAGE_KEY = "lc_lang";

/** Detecta el idioma inicial en el navegador: localStorage → navigator.language → EN. */
export function detectLang(): Lang {
  if (typeof window === "undefined") return "en";
  try {
    const stored = window.localStorage.getItem(LANG_STORAGE_KEY);
    if (stored === "es" || stored === "en") return stored;
  } catch {
    /* localStorage bloqueado */
  }
  const nav = window.navigator.language || "";
  return nav.toLowerCase().startsWith("es") ? "es" : "en";
}

/** Para metadata en el servidor a partir del header Accept-Language. */
export function langFromAcceptLanguage(header: string | null): Lang {
  if (!header) return "en";
  const first = header.split(",")[0]?.trim().toLowerCase() ?? "";
  return first.startsWith("es") ? "es" : "en";
}

const es = {
  meta: {
    title: "LedgerCore — el ledger de doble entrada para fintechs",
    description:
      "Ledger de doble entrada como servicio: idempotencia, holds, conciliación, webhooks firmados y aislamiento multi-tenant con RLS.",
  },
  nav: {
    blog: "Blog",
    docs: "Docs",
    quickstart: "Quickstart",
    login: "Iniciar sesión",
    trySandbox: "Probar el sandbox",
    ariaLabel: "Principal",
    home: "Inicio",
    homeAriaLabel: "LedgerCore — ir al inicio",
  },
  hero: {
    badge: "Ledger como servicio · en etapa temprana",
    titlePre: "El ledger de doble entrada para fintechs que ",
    titleHighlight: "mueven dinero",
    titlePost: "",
    subtitle:
      "¿Dónde está el dinero, a quién pertenece y qué operación explica cada centavo? LedgerCore responde esa pregunta en todo momento, con garantías de base de datos y no con hojas de cálculo.",
    ctaPrimary: "Probar el sandbox",
    ctaSecondary: "Ver la consola demo",
  },
  stats: [
    { value: "94%", label: "de las hojas de cálculo financieras contienen errores" },
    { value: "52%", label: "de las excepciones de conciliación nunca se explican del todo" },
    { value: "6 cifras", label: "los descuadres invisibles crecen en silencio hasta ser noticia" },
  ],
  statsFootPre: "El estado del dinero no puede vivir en columnas de Excel ni en un campo ",
  statsFootPost: " que alguien actualiza a mano.",
  scenarios: {
    answerLabel: "Qué hace LedgerCore:",
    title: "Cuatro escenarios que ya viviste",
    subtitle:
      "No son casos hipotéticos: son las noches que nos costaron construir esto. Si reconoces alguno, sabes exactamente para qué sirve LedgerCore.",
    items: [
      {
        role: "Finanzas / Cierre de mes",
        title: "El saldo no cuadra y nadie sabe desde cuándo",
        scenario:
          "El total de los wallets dice una cosa, el extracto del banco dice otra. La diferencia lleva semanas ahí y el cierre queda trabado mientras alguien revisa transacción por transacción.",
        answer:
          "Cada saldo es la suma de sus asientos, no una columna que alguien escribe. Puedes recalcularlo a cualquier fecha de corte y ver el punto exacto donde las dos series se separan.",
      },
      {
        role: "Compliance / Auditoría",
        title: "Te piden justificar un movimiento de hace ocho meses",
        scenario:
          "El auditor señala una línea vieja y pregunta qué la originó. El registro se actualizó tres veces desde entonces y el estado anterior no quedó en ningún lado.",
        answer:
          "El ledger es append-only: nada se edita, todo se corrige con un asiento nuevo. Cada centavo conserva su operación de origen y el histórico completo es reconstruible.",
      },
      {
        role: "Ingeniería / Plataforma",
        title: "El reintento acreditó dos veces al mismo cliente",
        scenario:
          "El proveedor no recibió tu 200, reintentó el callback y el depósito entró duplicado. Lo detectas días después, cuando el cliente ya retiró la diferencia.",
        answer:
          "La Idempotency-Key es parte del contrato, no una convención. El segundo POST devuelve la misma transacción en lugar de crear otra — el reintento deja de ser un riesgo.",
      },
      {
        role: "Soporte / Operaciones",
        title: "Hay fondos retenidos sin operación viva detrás",
        scenario:
          "El cliente reclama que su saldo disponible no le alcanza. Hay dinero reservado por una operación que nunca se completó ni se canceló, y liberarlo requiere tocar la base a mano.",
        answer:
          "Los holds son de primera clase: se reservan, expiran y se liberan con el mismo rigor contable que un posteo. El saldo disponible siempre es el posteado menos los holds vigentes, y cada uno es visible con su origen.",
      },
    ],
    footNote:
      "¿Tu escenario no está acá? Es exactamente lo que queremos escuchar — estamos en descubrimiento.",
  },
  how: {
    title: "Una API, asientos siempre balanceados",
    subtitle:
      "Un depósito de USD 100 con fee de 3: un solo POST, tres postings, y el invariante débitos = créditos verificado por el motor.",
    responseNote:
      "10000 = 9700 + 300. Montos en unidades menores (centavos), como enteros. Si no balancea, no se postea.",
    quickstartCta: "Hazlo tú mismo: de 0 a tu primera transacción en 10 minutos →",
  },
  features: {
    title: "Qué incluye",
    subtitle:
      "La infraestructura contable que cada fintech reconstruye a mano, lista como servicio.",
    items: [
      {
        title: "Doble entrada con idempotencia",
        body: "Cada transacción debita y acredita en el mismo movimiento atómico. La Idempotency-Key garantiza que un retry nunca duplica dinero.",
      },
      {
        title: "Holds y fondos reservados",
        body: "Reserva saldo antes de capturarlo. Autorizaciones, retenciones y liberaciones con el mismo rigor contable que un posteo.",
      },
      {
        title: "Conciliación integrada",
        body: "Ingiere extractos de bancos y PSPs, casa movimientos contra el ledger y expone cada discrepancia con su explicación.",
      },
      {
        title: "Webhooks firmados",
        body: "Eventos de posteo, hold y conciliación firmados criptográficamente, con reintentos y verificación de origen.",
      },
      {
        title: "Multi-tenant con RLS",
        body: "Aislamiento por tenant a nivel de fila en Postgres. Los datos de un cliente son invisibles para cualquier otro, por diseño.",
      },
      {
        title: "Consola de operaciones",
        body: "Dashboard para finanzas y soporte: balances, transacciones, salud de conciliación y credenciales de API en un solo lugar.",
      },
    ],
  },
  trust: [
    "Append-only por diseño",
    "Test anti-fuga entre tenants en CI",
    "Dinero como enteros, nunca floats",
  ],
  pricing: {
    title: "Cuánto cuesta",
    subtitle:
      "Todavía no hay lista de precios pública. Decirlo es más honesto que inventar una.",
    items: [
      {
        term: "El sandbox es gratis, y va a seguir siéndolo",
        body: "API completa, sin tarjeta. No es una prueba con fecha de vencimiento: es donde averiguas si esto te sirve.",
      },
      {
        term: "En producción se cobra por transacción asentada",
        body: "Nada más. Las cuentas, las lecturas de saldo y los webhooks no se miden. Si cobráramos por cuenta, te estaríamos penalizando por modelar bien tu dinero.",
      },
      {
        term: "Hoy trabajamos con un grupo chico de equipos",
        body: "El precio se define en esa conversación, contra lo que hoy te cuesta sostener tu ledger a mano. Si ya estás moviendo dinero en serio, hablemos.",
      },
    ],
  },
  finalCta: {
    title: "Deja de reconstruir el ledger. Constrúyelo una sola vez, bien.",
    subtitle:
      "Estamos en etapa de descubrimiento y trabajando de cerca con los primeros equipos. Prueba el sandbox y cuéntanos qué mueve tu dinero.",
  },
  footer: {
    tagline: "LedgerCore — infraestructura financiera",
  },
  blog: {
    metaTitle: "Blog · LedgerCore",
    title: "Blog",
    subtitle:
      "Notas de construir un ledger de doble entrada como servicio. Un post, una idea técnica.",
    readPost: "Leer el post",
    backToBlog: "Volver al blog",
    minRead: "min de lectura",
    viewSingular: "lectura",
    viewPlural: "lecturas",
  },
  comments: {
    title: "Comentarios",
    empty: "Todavía no hay comentarios. Si algo de acá te resuena o te parece equivocado, decilo.",
    nameLabel: "Tu nombre",
    namePlaceholder: "Tu nombre",
    bodyLabel: "Tu comentario",
    bodyPlaceholder: "¿Te pasó algo parecido? ¿Ves un error acá?",
    replyPlaceholder: "Tu respuesta…",
    send: "Comentar",
    sendReply: "Responder",
    sending: "Enviando…",
    reply: "Responder",
    cancelReply: "Cancelar respuesta",
    cancel: "Cancelar",
    footnote: "Se publica al instante. No pedimos correo ni guardamos tu IP.",
    errRateLimited: "Demasiados comentarios seguidos. Probá de nuevo en un rato.",
    errUnavailable: "Los comentarios no están disponibles en este momento.",
    errInvalid: "Revisá el nombre y el texto del comentario.",
    errNetwork: "No se pudo enviar. Revisá tu conexión.",
  },
  moderation: {
    metaTitle: "Moderación de comentarios · LedgerCore",
    title: "Moderación de comentarios",
    subtitle:
      "Todos los comentarios del blog, ocultos incluidos. Ocultar un comentario raíz también oculta sus respuestas; restaurarlo no las devuelve una por una.",
    tokenLabel: "Token de moderación",
    tokenPlaceholder: "Pegá el BLOG_ADMIN_TOKEN",
    tokenHint:
      "Se guarda solo en esta pestaña (sessionStorage) y viaja en el header Authorization.",
    enter: "Entrar",
    signOut: "Salir",
    errUnauthorized: "Token inválido.",
    errNotConfigured:
      "La moderación no está configurada: falta BLOG_ADMIN_TOKEN en el servidor.",
    errUnavailable: "No se pudo contactar la base de datos.",
    empty: "No hay comentarios todavía.",
    filterAll: "Todos",
    filterVisible: "Visibles",
    filterHidden: "Ocultos",
    statusVisible: "Visible",
    statusHidden: "Oculto",
    hide: "Ocultar",
    restore: "Restaurar",
    remove: "Eliminar",
    confirmRemove:
      "Se elimina de forma permanente, junto con sus respuestas. ¿Seguir?",
    replyTo: "en respuesta a",
    openPost: "Ver el post",
    total: "comentarios",
  },
  login: {
    tagline: "Consola de operaciones · ledger de doble partida como servicio",
    title: "Iniciar sesión",
    expired:
      "Tu sesión expiró o la llave dejó de ser válida. Vuelve a entrar con tu API key.",
    apiKeyLabel: "API key",
    apiKeyPlaceholder: "lk_sandbox_… o lk_live_…",
    rememberLabel: "Recordar en este dispositivo",
    rememberHint:
      "Guarda tu API key en el almacenamiento local del navegador para no volver a pegarla. Úsalo solo en equipos de confianza.",
    errInvalidKey:
      "API key inválida, revocada o de un tenant inactivo. Verifica que copiaste la llave completa (lk_…).",
    errNetwork: "No se pudo contactar la API. Verifica tu conexión e intenta de nuevo.",
    errGeneric: "No se pudo iniciar sesión. Intenta de nuevo.",
    submitBusy: "Verificando llave…",
    submit: "Entrar con API key",
    jwtNotePre:
      "La llave se intercambia por un JWT de 15 minutos que la consola renueva sola. ¿No tienes llave? ",
    jwtNoteLink: "Crea un sandbox gratis",
    or: "o",
    demoMode: "Entrar en modo demo",
  },
  signup: {
    metaTitle: "Crear sandbox · LedgerCore",
    tagline: "Sandbox gratuito · ledger de doble partida como servicio",
    createTitle: "Crear sandbox",
    days: "14 días",
    emailLabel: "Correo de trabajo",
    emailPlaceholder: "tu@empresa.com",
    companyLabel: "Nombre de la empresa",
    companyPlaceholder: "Acme Payments",
    submitBusy: "Creando sandbox…",
    submit: "Crear sandbox gratis",
    footnote:
      "Un sandbox por correo. El tenant y todos sus datos se eliminan automáticamente a los 14 días.",
    errEmailTaken: "Ese correo ya creó un tenant sandbox.",
    errLimit: "Se alcanzó el límite diario de registros. Intenta mañana.",
    errInvalid: "Datos inválidos.",
    errGeneric: "No se pudo crear el sandbox.",
    errNetwork: "No se pudo contactar la API. Intenta de nuevo.",
    readyTitle: "Sandbox listo",
    keyIntroPre: "Esta es tu API key. Se muestra ",
    keyIntroStrong: "una sola vez",
    keyIntroPost: ": guárdala ahora.",
    copied: "Copiada",
    copy: "Copiar",
    copiedCurl: "Copiado",
    demoIntro:
      "Flujo demo — token, ledger, cuentas y un depósito de 100.00 USD (10000 → 9700 wallet + 300 fee):",
    tenantPre: "Tenant",
    expiresPre: "expira el",
    purgeNote: "Después de esa fecha el tenant y sus datos se purgan automáticamente.",
    goConsole: "Ir a la consola",
    nextStepPre: "Siguiente paso:",
    nextStepLink: "el quickstart — tu primera transacción en 10 minutos",
    footerBrand: "LedgerCore — infraestructura financiera",
    haveAccount: "¿Ya tienes cuenta?",
    curlComments: {
      step1: "# 1. Token de acceso (JWT de 15 min)",
      step2: "# 2. Crear un ledger",
      step3: "# 3. Tres cuentas: caja (activo), wallet del cliente y comisiones",
      step4: "# 4. Deposito de 100.00 USD (10000 centavos): 9700 al wallet + 300 de fee",
    },
  },
};

const en: typeof es = {
  meta: {
    title: "LedgerCore — the double-entry ledger for fintechs",
    description:
      "Double-entry ledger as a service: idempotency, holds, reconciliation, signed webhooks, and multi-tenant isolation with RLS.",
  },
  nav: {
    blog: "Blog",
    docs: "Docs",
    quickstart: "Quickstart",
    login: "Sign in",
    trySandbox: "Try the sandbox",
    ariaLabel: "Main",
    home: "Home",
    homeAriaLabel: "LedgerCore — go to home",
  },
  hero: {
    badge: "Ledger as a service · early stage",
    titlePre: "The double-entry ledger for fintechs that ",
    titleHighlight: "move money",
    titlePost: "",
    subtitle:
      "Where is the money, who owns it, and which operation explains every cent? LedgerCore answers that question at all times — with database guarantees, not spreadsheets.",
    ctaPrimary: "Try the sandbox",
    ctaSecondary: "See the demo console",
  },
  stats: [
    { value: "94%", label: "of financial spreadsheets contain errors" },
    { value: "52%", label: "of reconciliation exceptions are never fully explained" },
    { value: "6 figures", label: "invisible discrepancies compound quietly until they make headlines" },
  ],
  statsFootPre: "The state of your money can't live in Excel columns or a ",
  statsFootPost: " field someone updates by hand.",
  scenarios: {
    answerLabel: "What LedgerCore does:",
    title: "Four scenarios you've already lived",
    subtitle:
      "These aren't hypotheticals — they're the nights that made us build this. If you recognize one, you already know what LedgerCore is for.",
    items: [
      {
        role: "Finance / Month-end close",
        title: "The balance doesn't match and nobody knows since when",
        scenario:
          "Wallet totals say one thing, the bank statement says another. The gap has been there for weeks, and the close is stuck while someone walks transaction by transaction.",
        answer:
          "Every balance is the sum of its entries, not a column someone writes. Recompute it as of any cutoff date and see the exact point where the two series diverge.",
      },
      {
        role: "Compliance / Audit",
        title: "You're asked to justify a movement from eight months ago",
        scenario:
          "The auditor points at an old line and asks what caused it. The record has been updated three times since, and the previous state was never kept anywhere.",
        answer:
          "The ledger is append-only: nothing is edited, everything is corrected with a new entry. Every cent keeps the operation that originated it, and the full history is reconstructable.",
      },
      {
        role: "Engineering / Platform",
        title: "The retry credited the same customer twice",
        scenario:
          "The provider never got your 200, retried the callback, and the deposit landed twice. You find out days later, once the customer has already withdrawn the difference.",
        answer:
          "The Idempotency-Key is part of the contract, not a convention. The second POST returns the same transaction instead of creating another one — a retry stops being a risk.",
      },
      {
        role: "Support / Operations",
        title: "Funds are held with no live operation behind them",
        scenario:
          "A customer reports their available balance is short. Money is reserved by an operation that never completed nor got cancelled, and releasing it means touching the database by hand.",
        answer:
          "Holds are first-class: reserved, expired, and released with the same accounting rigor as a posting. Available balance is always posted minus live holds, and each hold is visible with its origin.",
      },
    ],
    footNote:
      "Your scenario isn't here? That's exactly what we want to hear — we're in discovery.",
  },
  how: {
    title: "One API, always-balanced entries",
    subtitle:
      "A USD 100 deposit with a 3 fee: one POST, three postings, and the debits = credits invariant enforced by the engine.",
    responseNote:
      "10000 = 9700 + 300. Amounts in minor units (cents), as integers. If it doesn't balance, it doesn't post.",
    quickstartCta: "Do it yourself: from 0 to your first transaction in 10 minutes →",
  },
  features: {
    title: "What's included",
    subtitle:
      "The accounting infrastructure every fintech rebuilds by hand — ready as a service.",
    items: [
      {
        title: "Double-entry with idempotency",
        body: "Every transaction debits and credits in a single atomic move. The Idempotency-Key guarantees a retry never duplicates money.",
      },
      {
        title: "Holds and reserved funds",
        body: "Reserve balance before you capture it. Authorizations, holds, and releases with the same accounting rigor as a posting.",
      },
      {
        title: "Built-in reconciliation",
        body: "Ingest bank and PSP statements, match movements against the ledger, and surface every discrepancy with its explanation.",
      },
      {
        title: "Signed webhooks",
        body: "Posting, hold, and reconciliation events, cryptographically signed, with retries and origin verification.",
      },
      {
        title: "Multi-tenant with RLS",
        body: "Row-level tenant isolation in Postgres. One customer's data is invisible to every other — by design.",
      },
      {
        title: "Operations console",
        body: "A dashboard for finance and support: balances, transactions, reconciliation health, and API credentials in one place.",
      },
    ],
  },
  trust: [
    "Append-only by design",
    "Cross-tenant leak tests in CI",
    "Money as integers, never floats",
  ],
  pricing: {
    title: "What it costs",
    subtitle: "There's no public price list yet. Saying so beats inventing one.",
    items: [
      {
        term: "The sandbox is free, and it stays free",
        body: "Full API, no card. Not a trial with an expiry date — it's where you find out whether this is for you.",
      },
      {
        term: "In production you pay per posted transaction",
        body: "That's it. Accounts, balance reads and webhook deliveries aren't metered. Charging per account would penalise you for modelling your money properly.",
      },
      {
        term: "Right now we work with a small group of teams",
        body: "Price gets set in that conversation, against what your ledger costs you to maintain by hand today. If you're already moving real money, let's talk.",
      },
    ],
  },
  finalCta: {
    title: "Stop rebuilding the ledger. Build it once, and build it right.",
    subtitle:
      "We're in discovery and working closely with our first teams. Try the sandbox and tell us what moves your money.",
  },
  footer: {
    tagline: "LedgerCore — financial infrastructure",
  },
  blog: {
    metaTitle: "Blog · LedgerCore",
    title: "Blog",
    subtitle:
      "Notes from building a double-entry ledger as a service. One post, one technical idea.",
    readPost: "Read the post",
    backToBlog: "Back to the blog",
    minRead: "min read",
    viewSingular: "read",
    viewPlural: "reads",
  },
  comments: {
    title: "Comments",
    empty: "No comments yet. If something here resonates — or looks wrong — say so.",
    nameLabel: "Your name",
    namePlaceholder: "Your name",
    bodyLabel: "Your comment",
    bodyPlaceholder: "Have you hit something similar? See a mistake here?",
    replyPlaceholder: "Your reply…",
    send: "Post comment",
    sendReply: "Reply",
    sending: "Sending…",
    reply: "Reply",
    cancelReply: "Cancel reply",
    cancel: "Cancel",
    footnote: "Published instantly. We ask for no email and store no IP.",
    errRateLimited: "Too many comments in a row. Try again in a while.",
    errUnavailable: "Comments are unavailable right now.",
    errInvalid: "Check the name and the comment text.",
    errNetwork: "Could not send. Check your connection.",
  },
  moderation: {
    metaTitle: "Comment moderation · LedgerCore",
    title: "Comment moderation",
    subtitle:
      "Every blog comment, hidden ones included. Hiding a root comment also hides its replies; restoring it does not bring them back one by one.",
    tokenLabel: "Moderation token",
    tokenPlaceholder: "Paste the BLOG_ADMIN_TOKEN",
    tokenHint:
      "Kept in this tab only (sessionStorage) and sent in the Authorization header.",
    enter: "Enter",
    signOut: "Sign out",
    errUnauthorized: "Invalid token.",
    errNotConfigured:
      "Moderation is not configured: BLOG_ADMIN_TOKEN is missing on the server.",
    errUnavailable: "Could not reach the database.",
    empty: "No comments yet.",
    filterAll: "All",
    filterVisible: "Visible",
    filterHidden: "Hidden",
    statusVisible: "Visible",
    statusHidden: "Hidden",
    hide: "Hide",
    restore: "Restore",
    remove: "Delete",
    confirmRemove:
      "This permanently deletes it, along with its replies. Continue?",
    replyTo: "replying to",
    openPost: "Open the post",
    total: "comments",
  },
  login: {
    tagline: "Operations console · double-entry ledger as a service",
    title: "Sign in",
    expired:
      "Your session expired or the key is no longer valid. Sign in again with your API key.",
    apiKeyLabel: "API key",
    apiKeyPlaceholder: "lk_sandbox_… or lk_live_…",
    rememberLabel: "Remember on this device",
    rememberHint:
      "Stores your API key in the browser's local storage so you don't have to paste it again. Use only on trusted machines.",
    errInvalidKey:
      "Invalid or revoked API key, or an inactive tenant. Check that you copied the full key (lk_…).",
    errNetwork: "Could not reach the API. Check your connection and try again.",
    errGeneric: "Could not sign in. Please try again.",
    submitBusy: "Verifying key…",
    submit: "Sign in with API key",
    jwtNotePre:
      "The key is exchanged for a 15-minute JWT that the console renews on its own. No key yet? ",
    jwtNoteLink: "Create a free sandbox",
    or: "or",
    demoMode: "Enter demo mode",
  },
  signup: {
    metaTitle: "Create sandbox · LedgerCore",
    tagline: "Free sandbox · double-entry ledger as a service",
    createTitle: "Create sandbox",
    days: "14 days",
    emailLabel: "Work email",
    emailPlaceholder: "you@company.com",
    companyLabel: "Company name",
    companyPlaceholder: "Acme Payments",
    submitBusy: "Creating sandbox…",
    submit: "Create free sandbox",
    footnote:
      "One sandbox per email. The tenant and all its data are automatically deleted after 14 days.",
    errEmailTaken: "That email has already created a sandbox tenant.",
    errLimit: "The daily signup limit has been reached. Try again tomorrow.",
    errInvalid: "Invalid data.",
    errGeneric: "Could not create the sandbox.",
    errNetwork: "Could not reach the API. Please try again.",
    readyTitle: "Sandbox ready",
    keyIntroPre: "This is your API key. It's shown ",
    keyIntroStrong: "only once",
    keyIntroPost: " — save it now.",
    copied: "Copied",
    copy: "Copy",
    copiedCurl: "Copied",
    demoIntro:
      "Demo flow — token, ledger, accounts, and a 100.00 USD deposit (10000 → 9700 wallet + 300 fee):",
    tenantPre: "Tenant",
    expiresPre: "expires on",
    purgeNote: "After that date the tenant and its data are automatically purged.",
    goConsole: "Go to the console",
    nextStepPre: "Next step:",
    nextStepLink: "the quickstart — your first transaction in 10 minutes",
    footerBrand: "LedgerCore — financial infrastructure",
    haveAccount: "Already have an account?",
    curlComments: {
      step1: "# 1. Access token (15-min JWT)",
      step2: "# 2. Create a ledger",
      step3: "# 3. Three accounts: cash (asset), customer wallet, and fees",
      step4: "# 4. Deposit of 100.00 USD (10000 cents): 9700 to the wallet + 300 fee",
    },
  },
};

export const DICTIONARIES: Record<Lang, typeof es> = { es, en };

export type Dictionary = typeof es;
