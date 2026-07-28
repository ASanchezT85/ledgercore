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
    quickstart: "Quickstart",
    login: "Iniciar sesión",
    trySandbox: "Probar el sandbox",
    ariaLabel: "Principal",
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
  finalCta: {
    title: "Deja de reconstruir el ledger. Constrúyelo una sola vez, bien.",
    subtitle:
      "Estamos en etapa de descubrimiento y trabajando de cerca con los primeros equipos. Prueba el sandbox y cuéntanos qué mueve tu dinero.",
  },
  footer: {
    tagline: "LedgerCore — infraestructura financiera",
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
    quickstart: "Quickstart",
    login: "Sign in",
    trySandbox: "Try the sandbox",
    ariaLabel: "Main",
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
  finalCta: {
    title: "Stop rebuilding the ledger. Build it once, and build it right.",
    subtitle:
      "We're in discovery and working closely with our first teams. Try the sandbox and tell us what moves your money.",
  },
  footer: {
    tagline: "LedgerCore — financial infrastructure",
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
