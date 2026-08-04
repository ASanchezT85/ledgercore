import type { Lang } from "@/lib/i18n";

export type PostMeta = {
  slug: string;
  /** ISO date, used for <time> and OpenGraph. */
  dateISO: string;
  title: Record<Lang, string>;
  excerpt: Record<Lang, string>;
  /** Minutes, rounded — honest estimate, not engagement bait. */
  readingMinutes: number;
};

/**
 * The blog index and the per-post pages read from here, and the API routes use
 * it to reject view/comment writes for slugs that do not exist. Newest first.
 */
export const POSTS: PostMeta[] = [
  {
    slug: "holds-are-money",
    dateISO: "2026-08-04",
    readingMinutes: 6,
    title: {
      es: "Un hold es dinero, no una intención",
      en: "A hold is money, not an intention",
    },
    excerpt: {
      es: "El saldo disponible de un cliente no es lo que tiene: es lo que tiene menos lo que ya prometió. Tratar las reservas como un campo booleano deja plata congelada que nadie sabe liberar.",
      en: "A customer's available balance isn't what they have — it's what they have minus what they already promised. Treating reservations as a boolean field leaves money frozen that nobody knows how to release.",
    },
  },
  {
    slug: "a-balance-is-not-a-column",
    dateISO: "2026-08-01",
    readingMinutes: 7,
    title: {
      es: "Un saldo no es una columna",
      en: "A balance is not a column",
    },
    excerpt: {
      es: "En cuanto guardas el saldo en una columna, tienes dos fuentes de verdad y ninguna manera de saber cuál miente. La historia de una columna `balance` que se desincronizó y de cómo se detecta.",
      en: "The moment you store a balance in a column you have two sources of truth and no way to tell which one is lying. The story of a `balance` column that drifted, and how you catch it.",
    },
  },
  {
    slug: "idempotency-is-a-promise",
    dateISO: "2026-07-30",
    readingMinutes: 6,
    title: {
      es: "La idempotencia no es un header, es una promesa",
      en: "Idempotency is not a header, it's a promise",
    },
    excerpt: {
      es: "Aceptar una Idempotency-Key y no hacer nada con ella es peor que no aceptarla: el que integra cree que está protegido. Qué tiene que garantizar de verdad, y qué pasa en la carrera de dos reintentos simultáneos.",
      en: "Accepting an Idempotency-Key and doing nothing with it is worse than not accepting one: the integrator believes they're protected. What it must actually guarantee, and what happens when two retries race.",
    },
  },
  {
    slug: "the-six-figure-debt",
    dateISO: "2026-07-28",
    readingMinutes: 9,
    title: {
      es: "La deuda de seis cifras que no existía en ningún sistema",
      en: "The six-figure debt that existed in no system",
    },
    excerpt: {
      es: "Un proveedor de pagos dijo que le debíamos seis cifras. Nuestros sistemas decían que no le debíamos nada. Cinco cicatrices de mover dinero sobre tablas mutables, y el principio de ledger que grabó cada una.",
      en: "A payment provider said we owed them six figures. Our systems said we owed them nothing. Five scars from operating money movement on mutable tables, and the ledger principle each one burned in.",
    },
  },
];

export function isKnownPost(slug: string): boolean {
  return POSTS.some((p) => p.slug === slug);
}

export function findPost(slug: string): PostMeta | undefined {
  return POSTS.find((p) => p.slug === slug);
}

/** Locale-correct date label without pulling in a date library. */
export function formatPostDate(dateISO: string, lang: Lang): string {
  return new Date(`${dateISO}T00:00:00Z`).toLocaleDateString(
    lang === "es" ? "es-ES" : "en-US",
    { year: "numeric", month: "long", day: "numeric", timeZone: "UTC" },
  );
}
