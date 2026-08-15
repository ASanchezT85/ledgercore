"use client";

// Shared UI for the developer docs portal (/docs). Mirrors the visual
// language of the quickstart: same CodeBlock/ResponseBlock styling, same
// bilingual toggle (ES-first content), same header/footer chrome.

import { useState } from "react";
import Link from "next/link";
import { ArrowLeft, Check, Copy, Terminal } from "lucide-react";
import { LangToggle, LanguageProvider, useLang } from "@/components/language";
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";
import type { Lang } from "@/lib/i18n";

export const API_BASE = "https://api.ledgercore.sanchezavila.com";

// ---- content model --------------------------------------------------------

export type Block =
  | { kind: "code"; label: string; code: string }
  | { kind: "resp"; label: string; code: string }
  | { kind: "table"; head: string[]; rows: string[][] }
  | { kind: "list"; items: string[] };

export interface GuideSection {
  title: string;
  body?: string;
  blocks?: Block[];
}

export interface GuideCopy {
  badge: string;
  title: string;
  subtitle: string;
  sections: GuideSection[];
  backLabel: string;
  copy: string;
  copied: string;
}

export interface GuideContent {
  es: GuideCopy;
  en: GuideCopy;
}

// ---- building blocks ------------------------------------------------------

export function CodeBlock({
  label,
  code,
  copyLabel,
  copiedLabel,
}: {
  label: string;
  code: string;
  copyLabel: string;
  copiedLabel: string;
}) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="overflow-x-auto rounded-lg border border-edge bg-[#04070c] p-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="inline-flex items-center gap-2 text-[11px] font-medium tracking-wider text-ink-faint uppercase">
          <Terminal size={12} aria-hidden="true" />
          {label}
        </span>
        <button
          type="button"
          onClick={() => {
            void navigator.clipboard?.writeText(code).then(() => {
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            });
          }}
          className="inline-flex items-center gap-1 rounded-md border border-edge-strong px-2 py-1 text-[11px] font-medium text-ink-muted hover:text-accent"
        >
          {copied ? <Check size={11} aria-hidden="true" /> : <Copy size={11} aria-hidden="true" />}
          {copied ? copiedLabel : copyLabel}
        </button>
      </div>
      <pre className="font-mono text-xs leading-relaxed whitespace-pre text-ink-muted">
        <code>{code}</code>
      </pre>
    </div>
  );
}

export function ResponseBlock({ label, code }: { label: string; code: string }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-edge bg-surface-raised/60 p-4">
      <p className="mb-2 text-[11px] font-medium tracking-wider text-ink-faint uppercase">
        {label}
      </p>
      <pre className="font-mono text-xs leading-relaxed whitespace-pre text-accent/90">
        <code>{code}</code>
      </pre>
    </div>
  );
}

function DataTable({ head, rows }: { head: string[]; rows: string[][] }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-edge">
      <table className="w-full min-w-[560px] border-collapse text-left text-sm">
        <thead>
          <tr className="border-b border-edge bg-surface-raised/60">
            {head.map((h) => (
              <th
                key={h}
                className="px-3 py-2 text-[11px] font-medium tracking-wider text-ink-faint uppercase"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className="border-b border-edge/60 last:border-b-0">
              {row.map((cell, j) => (
                <td
                  key={j}
                  className={
                    "px-3 py-2 align-top text-ink-muted " +
                    (j === 0 ? "font-mono text-xs font-semibold text-accent" : "text-sm")
                  }
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function BlockView({
  block,
  copy,
  copied,
}: {
  block: Block;
  copy: string;
  copied: string;
}) {
  switch (block.kind) {
    case "code":
      return (
        <CodeBlock label={block.label} code={block.code} copyLabel={copy} copiedLabel={copied} />
      );
    case "resp":
      return <ResponseBlock label={block.label} code={block.code} />;
    case "table":
      return <DataTable head={block.head} rows={block.rows} />;
    case "list":
      return (
        <ul className="space-y-1.5 pl-1">
          {block.items.map((item) => (
            <li key={item} className="flex gap-2 text-sm leading-relaxed text-ink-muted">
              <span className="mt-[7px] size-1.5 shrink-0 rounded-full bg-accent/70" aria-hidden="true" />
              {item}
            </li>
          ))}
        </ul>
      );
  }
}

// ---- shell ----------------------------------------------------------------

function ShellChrome({
  backHref,
  backLabel,
  children,
}: {
  backHref?: string;
  backLabel?: string;
  children: React.ReactNode;
}) {
  const { lang } = useLang();
  return (
    <div className="min-h-screen">
      <header className="mx-auto flex w-full max-w-4xl items-center justify-between px-6 py-6">
        <div className="flex items-center gap-4">
          <Link href="/" aria-label="LedgerCore">
            <BrandLockup size={30} />
          </Link>
          {backHref && backLabel && (
            <Link
              href={backHref}
              className="inline-flex items-center gap-1.5 text-sm text-ink-muted transition-colors hover:text-accent"
            >
              <ArrowLeft size={14} aria-hidden="true" />
              {backLabel}
            </Link>
          )}
        </div>
        <LangToggle />
      </header>
      {children}
      <footer className="border-t border-edge px-6 py-6 text-center">
        <p className="text-[11px] tracking-wide text-ink-faint">
          LedgerCore — {lang === "es" ? "infraestructura financiera" : "financial infrastructure"}
        </p>
      </footer>
    </div>
  );
}

export function DocsShell({
  backHref,
  backLabel,
  children,
}: {
  backHref?: string;
  backLabel?: { es: string; en: string };
  children: React.ReactNode;
}) {
  return (
    <LanguageProvider>
      <ShellWithLang backHref={backHref} backLabel={backLabel}>
        {children}
      </ShellWithLang>
    </LanguageProvider>
  );
}

function ShellWithLang({
  backHref,
  backLabel,
  children,
}: {
  backHref?: string;
  backLabel?: { es: string; en: string };
  children: React.ReactNode;
}) {
  const { lang } = useLang();
  return (
    <ShellChrome backHref={backHref} backLabel={backLabel ? backLabel[lang] : undefined}>
      {children}
    </ShellChrome>
  );
}

// ---- generic guide renderer ----------------------------------------------

function GuideBody({ content }: { content: GuideContent }) {
  const { lang } = useLang();
  const c = content[lang as Lang];
  return (
    <main className="mx-auto w-full max-w-4xl px-6 pb-24">
      <Badge tone="emerald" className="mb-4">
        {c.badge}
      </Badge>
      <h1 className="max-w-2xl font-serif text-[34px] leading-tight tracking-[-0.01em] text-ink">{c.title}</h1>
      <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink-muted">{c.subtitle}</p>

      <div className="mt-12 space-y-12">
        {c.sections.map((section, i) => (
          <section key={section.title} className="relative pl-12">
            <span
              className="absolute top-0 left-0 flex size-8 items-center justify-center rounded-full border border-accent/40 bg-accent-soft font-mono text-sm font-semibold text-accent"
              aria-hidden="true"
            >
              {i + 1}
            </span>
            <h2 className="text-base font-semibold text-ink">{section.title}</h2>
            {section.body && (
              <p className="mt-1.5 max-w-2xl text-sm leading-relaxed text-ink-muted">
                {section.body}
              </p>
            )}
            {section.blocks && section.blocks.length > 0 && (
              <div className="mt-4 space-y-3">
                {section.blocks.map((block, j) => (
                  <BlockView key={j} block={block} copy={c.copy} copied={c.copied} />
                ))}
              </div>
            )}
          </section>
        ))}
      </div>
    </main>
  );
}

export function GuideView({ content }: { content: GuideContent }) {
  return (
    <LanguageProvider>
      <GuideShell content={content} />
    </LanguageProvider>
  );
}

function GuideShell({ content }: { content: GuideContent }) {
  const { lang } = useLang();
  const c = content[lang as Lang];
  return (
    <ShellChrome backHref="/docs" backLabel={c.backLabel}>
      <GuideBody content={content} />
    </ShellChrome>
  );
}
