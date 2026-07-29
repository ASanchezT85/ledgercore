"use client";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowRight,
  BookOpen,
  Braces,
  Coins,
  FileCode2,
  KeyRound,
  Layers,
  Package,
  RefreshCw,
  Rocket,
  Webhook,
} from "lucide-react";
import { LanguageProvider, LangToggle, useLang } from "@/components/language";
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";
import type { Lang } from "@/lib/i18n";

interface CardCopy {
  title: string;
  body: string;
}

interface IndexCopy {
  metaBadge: string;
  title: string;
  subtitle: string;
  start: CardCopy & { cta: string };
  api: CardCopy & { cta: string };
  sdks: CardCopy & { cta: string };
  guidesTitle: string;
  guidesSubtitle: string;
  guides: CardCopy[];
  footer: string;
}

const ES: IndexCopy = {
  metaBadge: "Developers",
  title: "Documentación para developers",
  subtitle:
    "Todo lo que necesitas para integrar el ledger de doble entrada: quickstart en 10 minutos, referencia completa de la API, guías de integración y SDKs oficiales de TypeScript y PHP.",
  start: {
    title: "Quickstart",
    body: "De 0 a tu primera transacción de doble partida en 10 minutos, con curls reales contra el sandbox y replay idempotente demostrado al final.",
    cta: "Empezar",
  },
  api: {
    title: "Referencia de API",
    body: "Los cuatro contratos OpenAPI — Identity, Ledger Core, Reconciliation y Webhooks — renderizados de forma interactiva. Son la fuente de verdad del API.",
    cta: "Explorar la API",
  },
  sdks: {
    title: "SDKs oficiales",
    body: "TypeScript (@ledgercore/sdk) y PHP (ledgercore/sdk): auth automática, Money helpers, idempotencia y autopaginación. Cero dependencias en runtime.",
    cta: "Ver SDKs",
  },
  guidesTitle: "Guías de integración",
  guidesSubtitle:
    "Los conceptos que sostienen cada endpoint: cómo se autentica, cómo viaja el dinero, por qué un retry nunca duplica fondos.",
  guides: [
    {
      title: "Autenticación",
      body: "API key → token exchange → JWT EdDSA de 15 minutos. Con curl, TypeScript y PHP.",
    },
    {
      title: "Dinero y montos",
      body: "Enteros en unidades menores, strings en JSON, Money helpers — y por qué nunca floats.",
    },
    {
      title: "Idempotencia",
      body: "idempotency_key, la cabecera X-Idempotent-Replay y la semántica exacta de los reintentos.",
    },
    {
      title: "Paginación",
      body: "limit / cursor / next_cursor, default 50 y máximo 200, y la autopaginación de los SDKs.",
    },
    {
      title: "Webhooks",
      body: "Suscripción, verificación de firma t=/v1= en TS y PHP, ventana anti-replay y rotación con 24 h de gracia.",
    },
    {
      title: "Errores",
      body: "El catálogo estable completo: código → HTTP → cuándo ocurre → qué hacer.",
    },
  ],
  footer: "infraestructura financiera",
};

const EN: IndexCopy = {
  metaBadge: "Developers",
  title: "Developer documentation",
  subtitle:
    "Everything you need to integrate the double-entry ledger: a 10-minute quickstart, the full API reference, integration guides and official TypeScript and PHP SDKs.",
  start: {
    title: "Quickstart",
    body: "From 0 to your first double-entry transaction in 10 minutes, with real curls against the sandbox and idempotent replay proven at the end.",
    cta: "Get started",
  },
  api: {
    title: "API reference",
    body: "The four OpenAPI contracts — Identity, Ledger Core, Reconciliation and Webhooks — rendered interactively. They are the API's source of truth.",
    cta: "Explore the API",
  },
  sdks: {
    title: "Official SDKs",
    body: "TypeScript (@ledgercore/sdk) and PHP (ledgercore/sdk): automatic auth, Money helpers, idempotency and auto-pagination. Zero runtime dependencies.",
    cta: "View SDKs",
  },
  guidesTitle: "Integration guides",
  guidesSubtitle:
    "The concepts behind every endpoint: how auth works, how money travels, why a retry never duplicates funds.",
  guides: [
    {
      title: "Authentication",
      body: "API key → token exchange → 15-minute EdDSA JWT. With curl, TypeScript and PHP.",
    },
    {
      title: "Money and amounts",
      body: "Integers in minor units, strings in JSON, Money helpers — and why never floats.",
    },
    {
      title: "Idempotency",
      body: "idempotency_key, the X-Idempotent-Replay header and the exact retry semantics.",
    },
    {
      title: "Pagination",
      body: "limit / cursor / next_cursor, default 50 and max 200, plus SDK auto-pagination.",
    },
    {
      title: "Webhooks",
      body: "Subscription, t=/v1= signature verification in TS and PHP, anti-replay window and rotation with a 24 h grace period.",
    },
    {
      title: "Errors",
      body: "The complete stable catalog: code → HTTP → when it happens → what to do.",
    },
  ],
  footer: "financial infrastructure",
};

const COPY: Record<Lang, IndexCopy> = { es: ES, en: EN };

const GUIDE_SLUGS = [
  "autenticacion",
  "dinero",
  "idempotencia",
  "paginacion",
  "webhooks",
  "errores",
] as const;

const GUIDE_ICONS = [KeyRound, Coins, RefreshCw, Layers, Webhook, AlertTriangle] as const;

function TopCard({
  href,
  icon: Icon,
  copy,
}: {
  href: string;
  icon: React.ComponentType<{ size?: number; className?: string; "aria-hidden"?: boolean }>;
  copy: CardCopy & { cta: string };
}) {
  return (
    <Link
      href={href}
      className="group flex flex-col rounded-(--radius-card) border border-edge bg-surface/70 p-6 transition-colors hover:border-accent/50"
    >
      <span className="mb-4 inline-flex size-10 items-center justify-center rounded-lg border border-accent/30 bg-accent-soft text-accent">
        <Icon size={18} aria-hidden={true} />
      </span>
      <h2 className="text-base font-semibold text-ink">{copy.title}</h2>
      <p className="mt-1.5 flex-1 text-sm leading-relaxed text-ink-muted">{copy.body}</p>
      <span className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-accent">
        {copy.cta}
        <ArrowRight
          size={14}
          className="transition-transform group-hover:translate-x-0.5"
          aria-hidden={true}
        />
      </span>
    </Link>
  );
}

function DocsIndexContent() {
  const { lang } = useLang();
  const c = COPY[lang];

  return (
    <div className="min-h-screen">
      <header className="mx-auto flex w-full max-w-5xl items-center justify-between px-6 py-6">
        <Link href="/" aria-label="LedgerCore">
          <BrandLockup size={30} />
        </Link>
        <LangToggle />
      </header>

      <main className="mx-auto w-full max-w-5xl px-6 pb-24">
        <Badge tone="emerald" className="mb-4">
          {c.metaBadge}
        </Badge>
        <h1 className="max-w-2xl text-3xl font-semibold tracking-tight text-ink">{c.title}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink-muted">{c.subtitle}</p>

        <div className="mt-10 grid gap-4 sm:grid-cols-3">
          <TopCard href="/docs/quickstart" icon={Rocket} copy={c.start} />
          <TopCard href="/docs/api" icon={Braces} copy={c.api} />
          <TopCard href="/docs/sdks" icon={Package} copy={c.sdks} />
        </div>

        <section className="mt-16">
          <div className="mb-6 flex items-center gap-2">
            <BookOpen size={18} className="text-accent" aria-hidden="true" />
            <h2 className="text-xl font-semibold tracking-tight text-ink">{c.guidesTitle}</h2>
          </div>
          <p className="mb-6 max-w-2xl text-sm leading-relaxed text-ink-muted">
            {c.guidesSubtitle}
          </p>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {c.guides.map((guide, i) => {
              const Icon = GUIDE_ICONS[i] ?? FileCode2;
              return (
                <Link
                  key={GUIDE_SLUGS[i]}
                  href={`/docs/guias/${GUIDE_SLUGS[i]}`}
                  className="group rounded-(--radius-card) border border-edge bg-surface/70 p-5 transition-colors hover:border-accent/50"
                >
                  <span className="mb-3 inline-flex size-8 items-center justify-center rounded-md border border-accent/30 bg-accent-soft text-accent">
                    <Icon size={15} aria-hidden={true} />
                  </span>
                  <h3 className="text-sm font-semibold text-ink group-hover:text-accent">
                    {guide.title}
                  </h3>
                  <p className="mt-1 text-[13px] leading-relaxed text-ink-muted">{guide.body}</p>
                </Link>
              );
            })}
          </div>
        </section>
      </main>

      <footer className="border-t border-edge px-6 py-6 text-center">
        <p className="text-[11px] tracking-wide text-ink-faint">LedgerCore — {c.footer}</p>
      </footer>
    </div>
  );
}

export function DocsIndexView() {
  return (
    <LanguageProvider>
      <DocsIndexContent />
    </LanguageProvider>
  );
}
