"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  Building2,
  Check,
  Copy,
  KeyRound,
  Mail,
  ShieldAlert,
  Sparkles,
} from "lucide-react";
import { ThinkingOrb } from "thinking-orbs";
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";
import { LanguageProvider, LangToggle, useLang } from "@/components/language";
import type { Dictionary } from "@/lib/i18n";

type SignupResult = {
  tenant_id: string;
  slug: string;
  api_key: string;
  key_prefix: string;
  expires_at: string;
};

function curlSnippet(apiKey: string, c: Dictionary["signup"]["curlComments"]): string {
  const base =
    typeof window !== "undefined" ? window.location.origin.replace(/:3000$/, ":8080") : "https://api.ledgercore.dev";
  return `${c.step1}
TOKEN=$(curl -s ${base}/v1/auth/token \\
  -H 'Content-Type: application/json' \\
  -d '{"api_key":"${apiKey}"}' | jq -r .access_token)
AUTH="Authorization: Bearer $TOKEN"; CT='Content-Type: application/json'

${c.step2}
LEDGER=$(curl -s ${base}/v1/ledgers -H "$AUTH" -H "$CT" \\
  -d '{"name":"main"}' | jq -r .id)

${c.step3}
CASH=$(curl -s ${base}/v1/accounts -H "$AUTH" -H "$CT" -d "{\\"ledger_id\\":\\"$LEDGER\\",
  \\"name\\":\\"assets:cash\\",\\"type\\":\\"asset\\",\\"normal_balance\\":\\"DEBIT\\"}" | jq -r .id)
WALLET=$(curl -s ${base}/v1/accounts -H "$AUTH" -H "$CT" -d "{\\"ledger_id\\":\\"$LEDGER\\",
  \\"name\\":\\"customer:c1:wallet\\",\\"type\\":\\"liability\\",\\"normal_balance\\":\\"CREDIT\\"}" | jq -r .id)
FEES=$(curl -s ${base}/v1/accounts -H "$AUTH" -H "$CT" -d "{\\"ledger_id\\":\\"$LEDGER\\",
  \\"name\\":\\"revenue:fees\\",\\"type\\":\\"revenue\\",\\"normal_balance\\":\\"CREDIT\\"}" | jq -r .id)

${c.step4}
curl -s ${base}/v1/transactions -H "$AUTH" -H "$CT" \\
  -H 'Idempotency-Key: demo-deposit-1' \\
  -d "{\\"ledger_id\\":\\"$LEDGER\\",\\"description\\":\\"first deposit\\",\\"status\\":\\"posted\\",
    \\"postings\\":[
      {\\"account_id\\":\\"$CASH\\",\\"direction\\":\\"DEBIT\\",\\"amount\\":{\\"asset\\":\\"USD\\",\\"amount\\":\\"10000\\"}},
      {\\"account_id\\":\\"$WALLET\\",\\"direction\\":\\"CREDIT\\",\\"amount\\":{\\"asset\\":\\"USD\\",\\"amount\\":\\"9700\\"}},
      {\\"account_id\\":\\"$FEES\\",\\"direction\\":\\"CREDIT\\",\\"amount\\":{\\"asset\\":\\"USD\\",\\"amount\\":\\"300\\"}}]}"`;
}

function SignupContent() {
  const { t, lang } = useLang();
  const s = t.signup;
  const [email, setEmail] = useState("");
  const [company, setCompany] = useState("");
  const [busy, setBusy] = useState(false);
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const [result, setResult] = useState<SignupResult | null>(null);
  const [copiedKey, setCopiedKey] = useState(false);
  const [copiedCurl, setCopiedCurl] = useState(false);

  const errorText = (key: string): string => {
    const messages: Record<string, string> = {
      email_taken: s.errEmailTaken,
      signup_limit_reached: s.errLimit,
      network: s.errNetwork,
    };
    return messages[key] ?? key;
  };

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErrorKey(null);
    try {
      const res = await fetch("/api/signup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, company_name: company }),
      });
      const data = await res.json();
      if (!res.ok) {
        if (data.error === "email_taken" || data.error === "signup_limit_reached") {
          setErrorKey(data.error);
        } else if (data.error === "validation_error") {
          setErrorKey(data.message ?? s.errInvalid);
        } else {
          setErrorKey(data.message ?? s.errGeneric);
        }
        return;
      }
      setResult(data as SignupResult);
    } catch {
      setErrorKey("network");
    } finally {
      setBusy(false);
    }
  }

  async function copy(text: string, mark: (v: boolean) => void) {
    try {
      await navigator.clipboard.writeText(text);
      mark(true);
      setTimeout(() => mark(false), 2000);
    } catch {
      /* clipboard unavailable; the user can select manually */
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className={result ? "w-full max-w-2xl" : "w-full max-w-sm"}>
        <div className="mb-8 flex flex-col items-center gap-3 text-center">
          <BrandLockup size={40} />
          <p className="text-sm text-ink-faint">{s.tagline}</p>
          <LangToggle />
        </div>

        <div className="rounded-(--radius-card) border border-edge bg-surface/80 p-6 shadow-[0_1px_0_rgb(255_255_255/0.04)_inset,0_16px_48px_rgb(2_6_12/0.6)] backdrop-blur-md">
          {!result ? (
            <>
              <div className="mb-5 flex items-center justify-between">
                <h1 className="text-sm font-semibold text-ink">{s.createTitle}</h1>
                <Badge tone="sky" className="gap-1">
                  <Sparkles size={11} aria-hidden="true" />
                  {s.days}
                </Badge>
              </div>

              <form onSubmit={submit} className="space-y-3">
                <label className="block">
                  <span className="mb-1.5 block text-xs font-medium text-ink-muted">
                    {s.emailLabel}
                  </span>
                  <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                    <Mail size={14} className="text-ink-faint" aria-hidden="true" />
                    <input
                      type="email"
                      required
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder={s.emailPlaceholder}
                      className="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
                    />
                  </span>
                </label>

                <label className="block">
                  <span className="mb-1.5 block text-xs font-medium text-ink-muted">
                    {s.companyLabel}
                  </span>
                  <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                    <Building2 size={14} className="text-ink-faint" aria-hidden="true" />
                    <input
                      type="text"
                      required
                      maxLength={255}
                      value={company}
                      onChange={(e) => setCompany(e.target.value)}
                      placeholder={s.companyPlaceholder}
                      className="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
                    />
                  </span>
                </label>

                {errorKey ? (
                  <p className="flex items-start gap-2 rounded-(--radius-control) border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-300">
                    <ShieldAlert size={14} className="mt-0.5 shrink-0" aria-hidden="true" />
                    {errorText(errorKey)}
                  </p>
                ) : null}

                <button
                  type="submit"
                  disabled={busy}
                  className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] shadow-[0_0_24px_rgb(16_185_129/0.3)] transition-colors hover:bg-accent disabled:opacity-60"
                >
                  {busy && (
                    <ThinkingOrb
                      state="working"
                      size={20}
                      theme="light"
                      aria-hidden="true"
                    />
                  )}
                  {busy ? s.submitBusy : s.submit}
                  {!busy && <ArrowRight size={15} aria-hidden="true" />}
                </button>
              </form>

              <p className="mt-4 text-center text-[11px] leading-relaxed text-ink-faint">
                {s.footnote}
              </p>
            </>
          ) : (
            <>
              <div className="mb-5 flex items-center justify-between">
                <h1 className="text-sm font-semibold text-ink">{s.readyTitle}</h1>
                <Badge tone="emerald" className="gap-1">
                  <Check size={11} aria-hidden="true" />
                  {result.slug}
                </Badge>
              </div>

              <p className="mb-2 text-xs text-ink-muted">
                {s.keyIntroPre}
                <span className="font-semibold text-ink">{s.keyIntroStrong}</span>
                {s.keyIntroPost}
              </p>
              <div className="mb-4 flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                <KeyRound size={14} className="shrink-0 text-ink-faint" aria-hidden="true" />
                <code className="w-full overflow-x-auto text-xs break-all text-ink">
                  {result.api_key}
                </code>
                <button
                  type="button"
                  onClick={() => copy(result.api_key, setCopiedKey)}
                  className="inline-flex shrink-0 items-center gap-1 rounded-(--radius-control) border border-edge-strong px-2 py-1 text-[11px] text-ink-muted transition-colors hover:text-ink"
                >
                  {copiedKey ? <Check size={12} aria-hidden="true" /> : <Copy size={12} aria-hidden="true" />}
                  {copiedKey ? s.copied : s.copy}
                </button>
              </div>

              <p className="mb-2 text-xs text-ink-muted">{s.demoIntro}</p>
              <div className="relative mb-4">
                <pre className="max-h-72 overflow-auto rounded-(--radius-control) border border-edge-strong bg-surface-raised p-3 text-[11px] leading-relaxed text-ink-muted">
                  {curlSnippet(result.api_key, s.curlComments)}
                </pre>
                <button
                  type="button"
                  onClick={() => copy(curlSnippet(result.api_key, s.curlComments), setCopiedCurl)}
                  className="absolute top-2 right-2 inline-flex items-center gap-1 rounded-(--radius-control) border border-edge-strong bg-surface px-2 py-1 text-[11px] text-ink-muted transition-colors hover:text-ink"
                >
                  {copiedCurl ? <Check size={12} aria-hidden="true" /> : <Copy size={12} aria-hidden="true" />}
                  {copiedCurl ? s.copiedCurl : s.copy}
                </button>
              </div>

              <p className="mb-4 text-[11px] text-ink-faint">
                {s.tenantPre} <code className="text-ink-muted">{result.tenant_id}</code> ·{" "}
                {s.expiresPre}{" "}
                {new Date(result.expires_at).toLocaleDateString(
                  lang === "es" ? "es" : "en-US",
                )}
                . {s.purgeNote}
              </p>

              <Link
                href="/dashboard"
                className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] shadow-[0_0_24px_rgb(16_185_129/0.3)] transition-colors hover:bg-accent"
              >
                {s.goConsole}
                <ArrowRight size={15} aria-hidden="true" />
              </Link>

              <p className="mt-4 text-center text-xs text-ink-muted">
                {s.nextStepPre}{" "}
                <Link
                  href="/docs/quickstart"
                  className="font-medium text-accent hover:underline"
                >
                  {s.nextStepLink}
                </Link>
              </p>
            </>
          )}
        </div>

        <p className="mt-6 text-center text-[11px] text-ink-faint">
          {s.footerBrand} ·{" "}
          <Link href="/login" className="underline decoration-edge underline-offset-2 hover:text-ink-muted">
            {s.haveAccount}
          </Link>
        </p>
      </div>
    </div>
  );
}

export default function SignupPage() {
  return (
    <LanguageProvider>
      <SignupContent />
    </LanguageProvider>
  );
}
