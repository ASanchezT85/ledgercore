"use client";

import Link from "next/link";
import {
  ArrowRight,
  BookLock,
  CheckCircle2,
  Layers,
  ShieldCheck,
} from "lucide-react";
import { LanguageProvider, useLang } from "@/components/language";
import { PublicFooter, PublicHeader } from "@/components/public-header";

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
      className="inline-flex h-11 items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep px-6 text-sm font-semibold text-[#03150e] transition-colors hover:bg-accent"
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
      <PublicHeader />

      {/* Hero */}
      <section className="mx-auto w-full max-w-6xl px-6 pt-16 pb-24 sm:pt-24">
        <p className="lc-enter mb-5 text-xs font-medium tracking-widest text-ink-faint uppercase">
          {t.hero.badge}
        </p>
        <h1 className="lc-enter max-w-3xl font-serif text-[42px] leading-[1.1] tracking-[-0.01em] text-ink [--lc-delay:60ms] sm:text-[56px]">
          {t.hero.titlePre}
          <span className="text-accent">{t.hero.titleHighlight}</span>
          {t.hero.titlePost}
        </h1>
        <p className="lc-enter mt-6 max-w-2xl text-lg leading-relaxed text-ink-muted [--lc-delay:120ms]">
          {t.hero.subtitle}
        </p>
        <div className="lc-enter mt-9 flex flex-wrap items-center gap-3 [--lc-delay:180ms]">
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
              className="rounded-(--radius-card) border border-edge bg-surface/70 p-6"
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
        <p className="mt-6 text-sm text-ink-faint">
          {t.statsFootPre}
          <code className="font-mono text-xs">balance</code>
          {t.statsFootPost}
        </p>
      </section>

      {/* Scenarios */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="lc-reveal mb-8 max-w-2xl">
          <h2 className="font-serif text-[30px] leading-tight tracking-[-0.01em] text-ink">
            {t.scenarios.title}
          </h2>
          <p className="mt-2 text-sm leading-relaxed text-ink-muted">
            {t.scenarios.subtitle}
          </p>
        </div>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {t.scenarios.items.map(({ role, title, scenario, answer }) => (
            <article
              key={title}
              className="flex flex-col rounded-(--radius-card) border border-edge bg-surface/70 p-6 transition-colors hover:border-accent/40"
            >
              <span className="text-[11px] font-medium tracking-widest text-ink-faint uppercase">
                {role}
              </span>
              <h3 className="mt-3 text-base font-semibold text-ink">{title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                {scenario}
              </p>
              <p className="mt-4 border-t border-edge pt-4 text-sm leading-relaxed text-ink">
                <span className="mr-2 font-semibold text-ink-faint">
                  {t.scenarios.answerLabel}
                </span>
                {answer}
              </p>
            </article>
          ))}
        </div>
        <p className="mt-6 text-sm text-ink-faint">
          {t.scenarios.footNote}
        </p>
      </section>

      {/* How it works */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="lc-reveal mb-8 max-w-2xl">
          <h2 className="font-serif text-[30px] leading-tight tracking-[-0.01em] text-ink">
            {t.how.title}
          </h2>
          <p className="mt-2 text-sm text-ink-muted">
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
        <p className="mt-6">
          <Link
            href="/docs/quickstart"
            className="text-sm font-medium text-accent hover:underline"
          >
            {t.how.quickstartCta}
          </Link>
        </p>
      </section>

      {/* Features */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="lc-reveal mb-8 max-w-2xl">
          <h2 className="font-serif text-[30px] leading-tight tracking-[-0.01em] text-ink">
            {t.features.title}
          </h2>
          <p className="mt-2 text-sm text-ink-muted">
            {t.features.subtitle}
          </p>
        </div>
        <div className="grid grid-cols-1 gap-x-12 gap-y-9 sm:grid-cols-2 lg:grid-cols-3">
          {t.features.items.map(({ title, body }) => (
            <div key={title} className="border-t border-edge-strong pt-4">
              <h3 className="text-base font-semibold text-ink">{title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                {body}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* Pricing */}
      <section className="mx-auto w-full max-w-6xl px-6 pb-20">
        <div className="lc-reveal mb-8 max-w-2xl">
          <h2 className="font-serif text-[30px] leading-tight tracking-[-0.01em] text-ink">
            {t.pricing.title}
          </h2>
          <p className="mt-2 text-sm text-ink-muted">{t.pricing.subtitle}</p>
        </div>
        <dl className="grid grid-cols-1 gap-x-10 gap-y-8 sm:grid-cols-2 lg:grid-cols-4">
          {t.pricing.tiers.map(({ name, price, included, overage }) => (
            <div key={name} className="border-t border-edge-strong pt-4">
              <dt className="text-[11px] font-medium tracking-widest text-ink-faint uppercase">
                {name}
              </dt>
              <dd>
                <p className="num mt-3 text-2xl font-semibold tracking-tight text-ink">
                  {price}
                </p>
                <p className="mt-3 text-sm leading-relaxed text-ink-muted">
                  {included}
                </p>
                <p className="mt-1 text-sm leading-relaxed text-ink-faint">
                  {overage}
                </p>
              </dd>
            </div>
          ))}
        </dl>
        <div className="mt-10 max-w-3xl space-y-2">
          {t.pricing.notes.map((note) => (
            <p key={note} className="text-sm leading-relaxed text-ink-faint">
              {note}
            </p>
          ))}
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
        <h2 className="mx-auto max-w-2xl font-serif text-[38px] leading-tight tracking-[-0.01em] text-ink">
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
      <PublicFooter />
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
