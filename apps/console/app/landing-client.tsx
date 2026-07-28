"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookLock,
  CheckCircle2,
  FileCheck2,
  Fingerprint,
  Layers,
  LayoutDashboard,
  Lock,
  Scale,
  ShieldCheck,
  Webhook,
} from "lucide-react";
import { BrandLockup } from "@/components/logo";
import { LanguageProvider, LangToggle, useLang } from "@/components/language";

const CURL_SNIPPET = `curl -X POST https://api.ledgercore.dev/v1/transactions \\
  -H "Authorization: Bearer $LEDGERCORE_API_KEY" \\
  -H "Idempotency-Key: dep_9f2c41d7" \\
  -H "Content-Type: application/json" \\
  -d '{
    "ledger": "main",
    "description": "Customer deposit — USD 100",
    "postings": [
      { "account": "assets/custody/bank/bofa",              "direction": "debit",  "amount": 10000, "asset": "USD" },
      { "account": "liabilities/customers/cus_8c31/wallet", "direction": "credit", "amount": 9700,  "asset": "USD" },
      { "account": "revenue/fees/deposit",                  "direction": "credit", "amount": 300,   "asset": "USD" }
    ]
  }'`;

const RESPONSE_SNIPPET = `{
  "id": "txn_01J9ZK3Q7W",
  "status": "posted",
  "balanced": true,
  "postings": [
    { "account": "assets/custody/bank/bofa",              "debit":  10000 },
    { "account": "liabilities/customers/cus_8c31/wallet", "credit":  9700 },
    { "account": "revenue/fees/deposit",                  "credit":   300 }
  ]
}`;

const FEATURE_ICONS = [
  Scale,
  Lock,
  FileCheck2,
  Webhook,
  Fingerprint,
  LayoutDashboard,
] as const;

const TRUST_ICONS = [BookLock, ShieldCheck, Layers] as const;

function CtaPrimary({
  href,
  children,
}: {
  href: string;
  children: React.ReactNode;
}) {
  return (
    <Link
      href={href}
      className="inline-flex h-11 items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep px-6 text-sm font-semibold text-[#03150e] shadow-[0_0_28px_rgb(16_185_129/0.35)] transition-colors hover:bg-accent"
    >
      {children}
      <ArrowRight size={15} aria-hidden="true" />
    </Link>
  );
}

function CtaSecondary({
  href,
  children,
}: {
  href: string;
  children: React.ReactNode;
}) {
  return (
    <Link
      href={href}
      className="inline-flex h-11 items-center justify-center gap-2 rounded-(--radius-control) border border-edge-strong px-6 text-sm font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent"
    >
      {children}
    </Link>
  );
}

function LandingContent() {
  const { t } = useLang();

  return (
    <div className="min-h-screen">
      {/* Header */}
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <BrandLockup size={30} />
        <nav className="flex items-center gap-2" aria-label={t.nav.ariaLabel}>
          <LangToggle />
          <Link
            href="/blog"
            className="rounded-(--radius-control) px-4 py-2 text-sm text-ink-muted transition-colors hover:text-ink"
          >
            {t.nav.blog}
          </Link>
          <Link
            href="/login"
            className="rounded-(--radius-control) px-4 py-2 text-sm text-ink-muted transition-colors hover:text-ink"
          >
            {t.nav.login}
          </Link>
          <Link
            href="/signup"
            className="rounded-(--radius-control) border border-edge-strong px-4 py-2 text-sm font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent"
          >
            {t.nav.trySandbox}
          </Link>
        </nav>
      </header>

      {/* Hero */}
      <section className="mx-auto w-full max-w-6xl px-6 pt-16 pb-20 text-center sm:pt-24">
        <p className="mx-auto mb-5 inline-flex items-center gap-2 rounded-full border border-edge bg-surface/70 px-4 py-1.5 text-xs font-medium tracking-wide text-accent">
          <span className="size-1.5 rounded-full bg-accent" aria-hidden="true" />
          {t.hero.badge}
        </p>
        <h1 className="mx-auto max-w-3xl text-4xl font-semibold tracking-tight text-ink sm:text-5xl sm:leading-[1.15]">
          {t.hero.titlePre}
          <span className="bg-gradient-to-r from-emerald-300 to-teal-300 bg-clip-text text-transparent">
            {t.hero.titleHighlight}
          </span>
          {t.hero.titlePost}
        </h1>
        <p className="mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-ink-muted">
          {t.hero.subtitle}
        </p>
        <div className="mt-9 flex flex-wrap items-center justify-center gap-3">
          <CtaPrimary href="/signup">{t.hero.ctaPrimary}</CtaPrimary>
          <CtaSecondary href="/dashboard">{t.hero.ctaSecondary}</CtaSecondary>
        </div>
      </section>

      {/* Problem stats */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          {t.stats.map((s) => (
            <div
              key={s.value}
              className="rounded-(--radius-card) border border-edge bg-surface/70 p-6 text-center"
            >
              <p className="num text-3xl font-semibold tracking-tight text-accent">
                {s.value}
              </p>
              <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                {s.label}
              </p>
            </div>
          ))}
        </div>
        <p className="mt-6 text-center text-sm text-ink-faint">
          {t.statsFootPre}
          <code className="font-mono text-xs">balance</code>
          {t.statsFootPost}
        </p>
      </section>

      {/* How it works */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="mb-8 text-center">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">
            {t.how.title}
          </h2>
          <p className="mx-auto mt-2 max-w-xl text-sm text-ink-muted">
            {t.how.subtitle}
          </p>
        </div>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-5">
          <div className="overflow-hidden rounded-(--radius-card) border border-edge bg-surface lg:col-span-3">
            <div className="flex items-center justify-between border-b border-edge px-4 py-2.5">
              <span className="font-mono text-[11px] text-ink-faint">
                POST /v1/transactions
              </span>
              <span className="text-[10px] tracking-widest text-ink-faint uppercase">
                request
              </span>
            </div>
            <pre className="overflow-x-auto p-4 font-mono text-[12px] leading-relaxed text-ink-muted">
              <code>{CURL_SNIPPET}</code>
            </pre>
          </div>
          <div className="overflow-hidden rounded-(--radius-card) border border-edge bg-surface lg:col-span-2">
            <div className="flex items-center justify-between border-b border-edge px-4 py-2.5">
              <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-accent">
                <CheckCircle2 size={12} aria-hidden="true" />
                201 Created · balanced
              </span>
              <span className="text-[10px] tracking-widest text-ink-faint uppercase">
                response
              </span>
            </div>
            <pre className="overflow-x-auto p-4 font-mono text-[12px] leading-relaxed text-ink-muted">
              <code>{RESPONSE_SNIPPET}</code>
            </pre>
            <p className="border-t border-edge px-4 py-3 text-[11px] leading-relaxed text-ink-faint">
              {t.how.responseNote}
            </p>
          </div>
        </div>
      </section>

      {/* Features */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="mb-8 text-center">
          <h2 className="text-2xl font-semibold tracking-tight text-ink">
            {t.features.title}
          </h2>
          <p className="mx-auto mt-2 max-w-xl text-sm text-ink-muted">
            {t.features.subtitle}
          </p>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {t.features.items.map(({ title, body }, i) => {
            const Icon = FEATURE_ICONS[i] ?? Scale;
            return (
              <div
                key={title}
                className="rounded-(--radius-card) border border-edge bg-surface/70 p-6 transition-colors hover:border-accent/40"
              >
                <span className="inline-flex size-9 items-center justify-center rounded-lg bg-accent-soft">
                  <Icon size={17} className="text-accent" aria-hidden="true" />
                </span>
                <h3 className="mt-4 text-sm font-semibold text-ink">{title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                  {body}
                </p>
              </div>
            );
          })}
        </div>
      </section>

      {/* Trust strip */}
      <section className="border-y border-edge bg-surface/50">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-center gap-x-10 gap-y-4 px-6 py-8">
          {t.trust.map((text, i) => {
            const Icon = TRUST_ICONS[i] ?? BookLock;
            return (
              <span
                key={text}
                className="inline-flex items-center gap-2.5 text-sm font-medium text-ink-muted"
              >
                <Icon size={16} className="text-accent" aria-hidden="true" />
                {text}
              </span>
            );
          })}
        </div>
      </section>

      {/* Final CTA */}
      <section className="mx-auto w-full max-w-6xl px-6 py-24 text-center">
        <h2 className="mx-auto max-w-2xl text-3xl font-semibold tracking-tight text-ink">
          {t.finalCta.title}
        </h2>
        <p className="mx-auto mt-4 max-w-xl text-sm leading-relaxed text-ink-muted">
          {t.finalCta.subtitle}
        </p>
        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
          <CtaPrimary href="/signup">{t.hero.ctaPrimary}</CtaPrimary>
          <CtaSecondary href="/dashboard">{t.hero.ctaSecondary}</CtaSecondary>
        </div>
      </section>

      {/* Footer */}
      <footer className="border-t border-edge">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-8">
          <BrandLockup size={22} />
          <p className="text-[11px] tracking-wide text-ink-faint">
            {t.footer.tagline}
          </p>
        </div>
      </footer>
    </div>
  );
}

export default function LandingClient() {
  return (
    <LanguageProvider>
      <LandingContent />
    </LanguageProvider>
  );
}
