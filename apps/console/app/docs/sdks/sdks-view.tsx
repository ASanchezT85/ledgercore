"use client";

import Link from "next/link";
import { ArrowLeft, ExternalLink, Github, Package } from "lucide-react";
import { LanguageProvider, LangToggle, useLang } from "@/components/language";
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";
import type { Lang } from "@/lib/i18n";
import { API_BASE, CodeBlock } from "../docs-ui";

const TS_INSTALL = `npm install @ledgercore/sdk`;

const TS_QUICKSTART = `import { LedgerCore, Money } from "@ledgercore/sdk";

const lc = new LedgerCore({ apiKey: "lk_sandbox_...", baseUrl: "${API_BASE}" });
const ledger = await lc.ledgers.create({ name: "main" });
const wallet = await lc.accounts.create({ ledger_id: ledger.id, name: "customer:42:wallet", type: "liability", normal_balance: "CREDIT" });
const txn = await lc.transactions.create({ ledger_id: ledger.id, postings: [/* debits = credits, Money.fromDecimal("100.50","USD",2) */] });`;

const PHP_INSTALL = `composer require ledgercore/sdk`;

const PHP_QUICKSTART = `use LedgerCore\\LedgerCore;
use LedgerCore\\Money;

$lc = new LedgerCore(['api_key' => 'lk_sandbox_...', 'base_url' => '${API_BASE}']);
$ledger = $lc->ledgers->create(['name' => 'main']);
$txn = $lc->transactions->create(['ledger_id' => $ledger['id'], 'postings' => [/* Money::fromDecimal('100.50','USD',2) */]]);`;

interface SdkLinks {
  registry: { label: string; href: string };
  github: { label: string; href: string };
}

const TS_LINKS: SdkLinks = {
  registry: {
    label: "npmjs.com/package/@ledgercore/sdk",
    href: "https://www.npmjs.com/package/@ledgercore/sdk",
  },
  github: {
    label: "github.com/ASanchezT85/ledgercore-sdk-typescript",
    href: "https://github.com/ASanchezT85/ledgercore-sdk-typescript",
  },
};

const PHP_LINKS: SdkLinks = {
  registry: {
    label: "packagist.org/packages/ledgercore/sdk",
    href: "https://packagist.org/packages/ledgercore/sdk",
  },
  github: {
    label: "github.com/ASanchezT85/ledgercore-sdk-php",
    href: "https://github.com/ASanchezT85/ledgercore-sdk-php",
  },
};

interface SdksCopy {
  badge: string;
  title: string;
  subtitle: string;
  back: string;
  tsTitle: string;
  tsBody: string;
  phpTitle: string;
  phpBody: string;
  install: string;
  quickstart: string;
  features: string[];
  linksLabel: string;
  copy: string;
  copied: string;
  footer: string;
}

const ES: SdksCopy = {
  badge: "SDKs oficiales",
  title: "SDKs: TypeScript y PHP",
  subtitle:
    "Los dos SDKs oficiales cubren toda la superficie de la API con la misma semántica: auth automática (token exchange + renovación), Money helpers sin redondeo silencioso, idempotencia de primera clase, autopaginación y verificación de firmas de webhook en tiempo constante.",
  back: "Docs",
  tsTitle: "TypeScript — @ledgercore/sdk",
  tsBody:
    "Node 18+ y navegadores, fetch nativo, cero dependencias en runtime. Tipado completo, ESM + CJS.",
  phpTitle: "PHP — ledgercore/sdk",
  phpBody:
    "PHP 8.1+, cURL nativo, cero dependencias obligatorias en runtime (el transporte es una interfaz: puedes enchufar tu propio cliente PSR-18).",
  install: "Instalación",
  quickstart: "Quickstart (5 líneas)",
  features: [
    "Auth gestionada: API key → JWT EdDSA de 15 min, caché, renovación <60 s y un retry en 401.",
    "Dinero siempre como strings de unidades menores: Money.fromDecimal / toDecimal, error explícito ante precisión imposible.",
    "Idempotencia: pasa tu key o el SDK genera un UUID v4; los retries nunca duplican dinero.",
    "Paginación: list({ limit, cursor }) + autopaginación listAll() en todas las colecciones.",
    "Webhooks: verificación de X-LedgerCore-Signature en tiempo constante, ventana anti-replay de 5 min, rotación con múltiples v1.",
    "Errores tipados con status, code y request_id sobre el catálogo estable.",
  ],
  linksLabel: "Paquete y código",
  copy: "Copiar",
  copied: "Copiado",
  footer: "infraestructura financiera",
};

const EN: SdksCopy = {
  badge: "Official SDKs",
  title: "SDKs: TypeScript and PHP",
  subtitle:
    "Both official SDKs cover the full API surface with identical semantics: automatic auth (token exchange + renewal), Money helpers with no silent rounding, first-class idempotency, auto-pagination and constant-time webhook signature verification.",
  back: "Docs",
  tsTitle: "TypeScript — @ledgercore/sdk",
  tsBody:
    "Node 18+ and browsers, native fetch, zero runtime dependencies. Fully typed, ESM + CJS.",
  phpTitle: "PHP — ledgercore/sdk",
  phpBody:
    "PHP 8.1+, native cURL, zero mandatory runtime dependencies (the transport is an interface — plug in your own PSR-18 client).",
  install: "Install",
  quickstart: "Quickstart (5 lines)",
  features: [
    "Auth handled for you: API key → 15-min EdDSA JWT, cached, renewed under 60 s, one retry on 401.",
    "Money always as minor-unit strings: Money.fromDecimal / toDecimal, explicit error on impossible precision.",
    "Idempotency: pass your key or the SDK generates a UUID v4; retries never duplicate money.",
    "Pagination: list({ limit, cursor }) plus listAll() auto-pagination on every collection.",
    "Webhooks: constant-time X-LedgerCore-Signature verification, 5-minute replay window, rotation with multiple v1 entries.",
    "Typed errors with status, code and request_id over the stable catalog.",
  ],
  linksLabel: "Package and source",
  copy: "Copy",
  copied: "Copied",
  footer: "financial infrastructure",
};

const COPY: Record<Lang, SdksCopy> = { es: ES, en: EN };

function SdkCard({
  title,
  body,
  install,
  quickstart,
  links,
  c,
}: {
  title: string;
  body: string;
  install: string;
  quickstart: string;
  links: SdkLinks;
  c: SdksCopy;
}) {
  return (
    <section className="rounded-(--radius-card) border border-edge bg-surface/70 p-6">
      <h2 className="text-lg font-semibold text-ink">{title}</h2>
      <p className="mt-1.5 max-w-2xl text-sm leading-relaxed text-ink-muted">{body}</p>
      <div className="mt-4 space-y-3">
        <CodeBlock label={c.install} code={install} copyLabel={c.copy} copiedLabel={c.copied} />
        <CodeBlock label={c.quickstart} code={quickstart} copyLabel={c.copy} copiedLabel={c.copied} />
      </div>
      <p className="mt-4 mb-2 text-[11px] font-medium tracking-wider text-ink-faint uppercase">
        {c.linksLabel}
      </p>
      <ul className="space-y-1.5">
        <li>
          <a
            href={links.registry.href}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 font-mono text-xs text-accent hover:underline"
          >
            <Package size={13} aria-hidden="true" />
            {links.registry.label}
            <ExternalLink size={11} aria-hidden="true" />
          </a>
        </li>
        <li>
          <a
            href={links.github.href}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 font-mono text-xs text-accent hover:underline"
          >
            <Github size={13} aria-hidden="true" />
            {links.github.label}
            <ExternalLink size={11} aria-hidden="true" />
          </a>
        </li>
      </ul>
    </section>
  );
}

function SdksContent() {
  const { lang } = useLang();
  const c = COPY[lang];

  return (
    <div className="min-h-screen">
      <header className="mx-auto flex w-full max-w-4xl items-center justify-between px-6 py-6">
        <div className="flex items-center gap-4">
          <Link href="/" aria-label="LedgerCore">
            <BrandLockup size={30} />
          </Link>
          <Link
            href="/docs"
            className="inline-flex items-center gap-1.5 text-sm text-ink-muted transition-colors hover:text-accent"
          >
            <ArrowLeft size={14} aria-hidden="true" />
            {c.back}
          </Link>
        </div>
        <LangToggle />
      </header>

      <main className="mx-auto w-full max-w-4xl px-6 pb-24">
        <Badge tone="emerald" className="mb-4">
          {c.badge}
        </Badge>
        <h1 className="max-w-2xl font-serif text-[34px] leading-tight tracking-[-0.01em] text-ink">{c.title}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink-muted">{c.subtitle}</p>

        <ul className="mt-6 space-y-1.5">
          {c.features.map((f) => (
            <li key={f} className="flex gap-2 text-sm leading-relaxed text-ink-muted">
              <span className="mt-[7px] size-1.5 shrink-0 rounded-full bg-accent/70" aria-hidden="true" />
              {f}
            </li>
          ))}
        </ul>

        <div className="mt-10 space-y-8">
          <SdkCard
            title={c.tsTitle}
            body={c.tsBody}
            install={TS_INSTALL}
            quickstart={TS_QUICKSTART}
            links={TS_LINKS}
            c={c}
          />
          <SdkCard
            title={c.phpTitle}
            body={c.phpBody}
            install={PHP_INSTALL}
            quickstart={PHP_QUICKSTART}
            links={PHP_LINKS}
            c={c}
          />
        </div>
      </main>

      <footer className="border-t border-edge px-6 py-6 text-center">
        <p className="text-[11px] tracking-wide text-ink-faint">LedgerCore — {c.footer}</p>
      </footer>
    </div>
  );
}

export function SdksView() {
  return (
    <LanguageProvider>
      <SdksContent />
    </LanguageProvider>
  );
}
