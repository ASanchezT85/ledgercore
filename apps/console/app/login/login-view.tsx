"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowRight, FlaskConical, KeyRound } from "lucide-react";
import { ThinkingOrb } from "thinking-orbs";
import { BrandLockup } from "@/components/logo";
import { AuthError, login } from "@/lib/session";

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const [apiKey, setApiKey] = useState("");
  const [remember, setRemember] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const expired = params.get("reason") === "expired";

  async function onSubmit(event: React.FormEvent) {
    event.preventDefault();
    if (!apiKey.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      await login(apiKey, remember);
      router.push("/dashboard");
    } catch (err) {
      if (err instanceof AuthError) {
        setError(
          "API key inválida, revocada o de un tenant inactivo. Verifica que copiaste la llave completa (lk_…).",
        );
      } else if (err instanceof Error && err.message === "network") {
        setError("No se pudo contactar la API. Verifica tu conexión e intenta de nuevo.");
      } else {
        setError("No se pudo iniciar sesión. Intenta de nuevo.");
      }
      setBusy(false);
    }
  }

  return (
    <div className="rounded-(--radius-card) border border-edge bg-surface/80 p-6 shadow-[0_1px_0_rgb(255_255_255/0.04)_inset,0_16px_48px_rgb(2_6_12/0.6)] backdrop-blur-md">
      <h1 className="mb-5 text-sm font-semibold text-ink">Iniciar sesión</h1>

      {expired && !error && (
        <p
          role="status"
          className="mb-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-ink"
        >
          Tu sesión expiró o la llave dejó de ser válida. Vuelve a entrar con tu
          API key.
        </p>
      )}

      <form onSubmit={onSubmit} className="space-y-3">
        <label className="block">
          <span className="mb-1.5 block text-xs font-medium text-ink-muted">
            API key
          </span>
          <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5 focus-within:border-accent/60">
            <KeyRound size={14} className="shrink-0 text-ink-faint" aria-hidden="true" />
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="lk_sandbox_… o lk_live_…"
              className="w-full bg-transparent font-mono text-sm text-ink outline-none placeholder:font-sans placeholder:text-ink-faint"
              autoComplete="off"
              spellCheck={false}
              aria-label="API key"
            />
          </span>
        </label>

        <label className="flex items-start gap-2">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => setRemember(e.target.checked)}
            className="mt-0.5 accent-emerald-500"
          />
          <span className="text-xs text-ink-muted">
            Recordar en este dispositivo
            <span className="mt-0.5 block text-[11px] text-ink-faint">
              Guarda tu API key en el almacenamiento local del navegador para
              no volver a pegarla. Úsalo solo en equipos de confianza.
            </span>
          </span>
        </label>

        {error && (
          <p role="alert" className="rounded-lg border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-ink">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={busy || apiKey.trim().length === 0}
          className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] shadow-[0_0_24px_rgb(16_185_129/0.3)] transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
        >
          {busy && (
            <ThinkingOrb
              state="listening"
              size={20}
              theme="light"
              aria-hidden="true"
            />
          )}
          {busy ? "Verificando llave…" : "Entrar con API key"}
          {!busy && <ArrowRight size={15} aria-hidden="true" />}
        </button>
      </form>

      <p className="mt-3 text-center text-[11px] text-ink-faint">
        La llave se intercambia por un JWT de 15 minutos que la consola renueva
        sola. ¿No tienes llave?{" "}
        <Link href="/signup" className="text-accent hover:underline">
          Crea un sandbox gratis
        </Link>
        .
      </p>

      <div className="my-5 flex items-center gap-3">
        <span className="h-px flex-1 bg-edge" aria-hidden="true" />
        <span className="text-[10px] tracking-widest text-ink-faint uppercase">o</span>
        <span className="h-px flex-1 bg-edge" aria-hidden="true" />
      </div>

      <Link
        href="/dashboard"
        className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) border border-edge-strong text-sm font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent"
      >
        <FlaskConical size={14} aria-hidden="true" />
        Entrar en modo demo
      </Link>
    </div>
  );
}

export function LoginView() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center gap-3 text-center">
          <BrandLockup size={40} />
          <p className="text-sm text-ink-faint">
            Consola de operaciones · ledger de doble partida como servicio
          </p>
        </div>

        <Suspense fallback={null}>
          <LoginForm />
        </Suspense>

        <p className="mt-6 text-center text-[11px] text-ink-faint">
          LedgerCore — infraestructura financiera
        </p>
      </div>
    </div>
  );
}
