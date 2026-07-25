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
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";

type SignupResult = {
  tenant_id: string;
  slug: string;
  api_key: string;
  key_prefix: string;
  expires_at: string;
};

function curlSnippet(apiKey: string): string {
  const base =
    typeof window !== "undefined" ? window.location.origin.replace(/:3000$/, ":8080") : "https://api.ledgercore.dev";
  return `# 1. Token de acceso (JWT de 15 min)
TOKEN=$(curl -s ${base}/v1/auth/token \\
  -H 'Content-Type: application/json' \\
  -d '{"api_key":"${apiKey}"}' | jq -r .access_token)
AUTH="Authorization: Bearer $TOKEN"; CT='Content-Type: application/json'

# 2. Crear un ledger
LEDGER=$(curl -s ${base}/v1/ledgers -H "$AUTH" -H "$CT" \\
  -d '{"name":"main"}' | jq -r .id)

# 3. Tres cuentas: caja (activo), wallet del cliente y comisiones
CASH=$(curl -s ${base}/v1/accounts -H "$AUTH" -H "$CT" -d "{\\"ledger_id\\":\\"$LEDGER\\",
  \\"name\\":\\"assets:cash\\",\\"type\\":\\"asset\\",\\"normal_balance\\":\\"DEBIT\\"}" | jq -r .id)
WALLET=$(curl -s ${base}/v1/accounts -H "$AUTH" -H "$CT" -d "{\\"ledger_id\\":\\"$LEDGER\\",
  \\"name\\":\\"customer:c1:wallet\\",\\"type\\":\\"liability\\",\\"normal_balance\\":\\"CREDIT\\"}" | jq -r .id)
FEES=$(curl -s ${base}/v1/accounts -H "$AUTH" -H "$CT" -d "{\\"ledger_id\\":\\"$LEDGER\\",
  \\"name\\":\\"revenue:fees\\",\\"type\\":\\"revenue\\",\\"normal_balance\\":\\"CREDIT\\"}" | jq -r .id)

# 4. Deposito de 100.00 USD (10000 centavos): 9700 al wallet + 300 de fee
curl -s ${base}/v1/transactions -H "$AUTH" -H "$CT" \\
  -H 'Idempotency-Key: demo-deposit-1' \\
  -d "{\\"ledger_id\\":\\"$LEDGER\\",\\"description\\":\\"first deposit\\",\\"status\\":\\"posted\\",
    \\"postings\\":[
      {\\"account_id\\":\\"$CASH\\",\\"direction\\":\\"DEBIT\\",\\"amount\\":{\\"asset\\":\\"USD\\",\\"amount\\":\\"10000\\"}},
      {\\"account_id\\":\\"$WALLET\\",\\"direction\\":\\"CREDIT\\",\\"amount\\":{\\"asset\\":\\"USD\\",\\"amount\\":\\"9700\\"}},
      {\\"account_id\\":\\"$FEES\\",\\"direction\\":\\"CREDIT\\",\\"amount\\":{\\"asset\\":\\"USD\\",\\"amount\\":\\"300\\"}}]}"`;
}

export default function SignupPage() {
  const [email, setEmail] = useState("");
  const [company, setCompany] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<SignupResult | null>(null);
  const [copiedKey, setCopiedKey] = useState(false);
  const [copiedCurl, setCopiedCurl] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/signup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, company_name: company }),
      });
      const data = await res.json();
      if (!res.ok) {
        const messages: Record<string, string> = {
          email_taken: "Ese correo ya creó un tenant sandbox.",
          signup_limit_reached:
            "Se alcanzó el límite diario de registros. Intenta mañana.",
          validation_error: data.message ?? "Datos inválidos.",
        };
        setError(messages[data.error] ?? data.message ?? "No se pudo crear el sandbox.");
        return;
      }
      setResult(data as SignupResult);
    } catch {
      setError("No se pudo contactar la API. Intenta de nuevo.");
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
          <p className="text-sm text-ink-faint">
            Sandbox gratuito · ledger de doble partida como servicio
          </p>
        </div>

        <div className="rounded-(--radius-card) border border-edge bg-surface/80 p-6 shadow-[0_1px_0_rgb(255_255_255/0.04)_inset,0_16px_48px_rgb(2_6_12/0.6)] backdrop-blur-md">
          {!result ? (
            <>
              <div className="mb-5 flex items-center justify-between">
                <h1 className="text-sm font-semibold text-ink">Crear sandbox</h1>
                <Badge tone="sky" className="gap-1">
                  <Sparkles size={11} aria-hidden="true" />
                  14 días
                </Badge>
              </div>

              <form onSubmit={submit} className="space-y-3">
                <label className="block">
                  <span className="mb-1.5 block text-xs font-medium text-ink-muted">
                    Correo de trabajo
                  </span>
                  <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                    <Mail size={14} className="text-ink-faint" aria-hidden="true" />
                    <input
                      type="email"
                      required
                      value={email}
                      onChange={(e) => setEmail(e.target.value)}
                      placeholder="tu@empresa.com"
                      className="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
                    />
                  </span>
                </label>

                <label className="block">
                  <span className="mb-1.5 block text-xs font-medium text-ink-muted">
                    Nombre de la empresa
                  </span>
                  <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                    <Building2 size={14} className="text-ink-faint" aria-hidden="true" />
                    <input
                      type="text"
                      required
                      maxLength={255}
                      value={company}
                      onChange={(e) => setCompany(e.target.value)}
                      placeholder="Acme Payments"
                      className="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
                    />
                  </span>
                </label>

                {error ? (
                  <p className="flex items-start gap-2 rounded-(--radius-control) border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-300">
                    <ShieldAlert size={14} className="mt-0.5 shrink-0" aria-hidden="true" />
                    {error}
                  </p>
                ) : null}

                <button
                  type="submit"
                  disabled={busy}
                  className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] shadow-[0_0_24px_rgb(16_185_129/0.3)] transition-colors hover:bg-accent disabled:opacity-60"
                >
                  {busy ? "Creando sandbox…" : "Crear sandbox gratis"}
                  <ArrowRight size={15} aria-hidden="true" />
                </button>
              </form>

              <p className="mt-4 text-center text-[11px] leading-relaxed text-ink-faint">
                Un sandbox por correo. El tenant y todos sus datos se eliminan
                automáticamente a los 14 días.
              </p>
            </>
          ) : (
            <>
              <div className="mb-5 flex items-center justify-between">
                <h1 className="text-sm font-semibold text-ink">Sandbox listo</h1>
                <Badge tone="emerald" className="gap-1">
                  <Check size={11} aria-hidden="true" />
                  {result.slug}
                </Badge>
              </div>

              <p className="mb-2 text-xs text-ink-muted">
                Esta es tu API key. Se muestra{" "}
                <span className="font-semibold text-ink">una sola vez</span>:
                guárdala ahora.
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
                  {copiedKey ? "Copiada" : "Copiar"}
                </button>
              </div>

              <p className="mb-2 text-xs text-ink-muted">
                Flujo demo — token, ledger, cuentas y un depósito de 100.00 USD
                (10000 → 9700 wallet + 300 fee):
              </p>
              <div className="relative mb-4">
                <pre className="max-h-72 overflow-auto rounded-(--radius-control) border border-edge-strong bg-surface-raised p-3 text-[11px] leading-relaxed text-ink-muted">
                  {curlSnippet(result.api_key)}
                </pre>
                <button
                  type="button"
                  onClick={() => copy(curlSnippet(result.api_key), setCopiedCurl)}
                  className="absolute top-2 right-2 inline-flex items-center gap-1 rounded-(--radius-control) border border-edge-strong bg-surface px-2 py-1 text-[11px] text-ink-muted transition-colors hover:text-ink"
                >
                  {copiedCurl ? <Check size={12} aria-hidden="true" /> : <Copy size={12} aria-hidden="true" />}
                  {copiedCurl ? "Copiado" : "Copiar"}
                </button>
              </div>

              <p className="mb-4 text-[11px] text-ink-faint">
                Tenant <code className="text-ink-muted">{result.tenant_id}</code> · expira el{" "}
                {new Date(result.expires_at).toLocaleDateString()}. Después de esa
                fecha el tenant y sus datos se purgan automáticamente.
              </p>

              <Link
                href="/"
                className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] shadow-[0_0_24px_rgb(16_185_129/0.3)] transition-colors hover:bg-accent"
              >
                Ir a la consola
                <ArrowRight size={15} aria-hidden="true" />
              </Link>
            </>
          )}
        </div>

        <p className="mt-6 text-center text-[11px] text-ink-faint">
          LedgerCore — infraestructura financiera ·{" "}
          <Link href="/login" className="underline decoration-edge underline-offset-2 hover:text-ink-muted">
            ¿Ya tienes cuenta?
          </Link>
        </p>
      </div>
    </div>
  );
}
