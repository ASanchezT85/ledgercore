"use client";

/**
 * Shared plumbing for pages that render real tenant data when a session
 * exists and fall back to the demo dataset otherwise.
 */

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { CloudOff, Copy, Check, Sparkles } from "lucide-react";
import { ThinkingOrb, type OrbState } from "thinking-orbs";
import { Card } from "@/components/ui/card";
import { ApiDownError, AuthError } from "@/lib/live";
import { hasSession, logout } from "@/lib/session";

export type LiveState<T> =
  | { status: "loading" }
  | { status: "demo" }
  | { status: "live"; data: T }
  | { status: "down" }
  | { status: "error"; message: string };

/**
 * Runs `loader` when a real session exists; resolves to "demo" otherwise.
 * On 401 the session is cleared and the user is sent back to /login.
 */
export function useLiveData<T>(loader: () => Promise<T>): LiveState<T> & {
  reload: () => void;
} {
  const router = useRouter();
  const [state, setState] = useState<LiveState<T>>({ status: "loading" });
  const [tick, setTick] = useState(0);
  const reload = useCallback(() => setTick((n) => n + 1), []);

  useEffect(() => {
    let cancelled = false;
    if (!hasSession()) {
      setState({ status: "demo" });
      return;
    }
    setState({ status: "loading" });
    loader().then(
      (data) => {
        if (!cancelled) setState({ status: "live", data });
      },
      (err: unknown) => {
        if (cancelled) return;
        if (err instanceof AuthError) {
          logout();
          router.replace("/login?reason=expired");
          return;
        }
        if (err instanceof ApiDownError) {
          setState({ status: "down" });
          return;
        }
        setState({
          status: "error",
          message: err instanceof Error ? err.message : "unknown_error",
        });
      },
    );
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick]);

  return { ...state, reload };
}

/**
 * Loading state for live pages. The orb animation is a real-wait indicator
 * only (brand rule: never decorative). The package is canvas-based and
 * client-only internally (paints in an effect, resolves dark/light itself)
 * and freezes the animation under prefers-reduced-motion; the text label
 * below stays as the always-available fallback.
 */
export function LoadingCard({ orb = "searching" }: { orb?: OrbState }) {
  return (
    <Card>
      <div className="flex flex-col items-center gap-3 py-10 text-sm text-ink-faint">
        <ThinkingOrb state={orb} size={64} aria-hidden="true" />
        Cargando datos del tenant…
      </div>
    </Card>
  );
}

export function ApiDownBanner({ onRetry }: { onRetry: () => void }) {
  return (
    <div
      role="alert"
      className="flex items-center justify-between gap-4 rounded-(--radius-card) border border-danger/40 bg-danger/10 px-5 py-4"
    >
      <div className="flex items-center gap-3">
        <CloudOff size={18} className="shrink-0 text-danger" aria-hidden="true" />
        <div>
          <p className="text-sm font-medium text-ink">
            No se pudo contactar la API de LedgerCore
          </p>
          <p className="text-xs text-ink-faint">
            Tu sesión sigue activa; los datos volverán cuando la API responda.
          </p>
        </div>
      </div>
      <button
        type="button"
        onClick={onRetry}
        className="h-8 shrink-0 rounded-(--radius-control) border border-edge-strong px-3 text-xs font-medium text-ink hover:border-accent/60 hover:text-accent"
      >
        Reintentar
      </button>
    </div>
  );
}

export function ErrorBanner({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div
      role="alert"
      className="flex items-center justify-between gap-4 rounded-(--radius-card) border border-warning/40 bg-warning/10 px-5 py-4"
    >
      <p className="text-sm text-ink">
        La API respondió con un error:{" "}
        <code className="font-mono text-xs text-warning">{message}</code>
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="h-8 shrink-0 rounded-(--radius-control) border border-edge-strong px-3 text-xs font-medium text-ink hover:border-accent/60 hover:text-accent"
      >
        Reintentar
      </button>
    </div>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
      className="inline-flex items-center gap-1 rounded-md border border-edge-strong px-2 py-1 text-[11px] font-medium text-ink-muted hover:text-accent"
    >
      {copied ? <Check size={11} aria-hidden="true" /> : <Copy size={11} aria-hidden="true" />}
      {copied ? "Copiado" : "Copiar"}
    </button>
  );
}

/**
 * Dignified empty state for a fresh tenant: what this page will show and the
 * exact curl to create the first data point.
 */
export function EmptyState({
  title,
  body,
  curl,
  curlLabel,
}: {
  title: string;
  body: string;
  curl?: string;
  curlLabel?: string;
}) {
  return (
    <Card>
      <div className="flex flex-col items-center gap-3 py-8 text-center">
        <span className="flex size-10 items-center justify-center rounded-full bg-accent-soft text-accent">
          <Sparkles size={18} aria-hidden="true" />
        </span>
        <p className="text-sm font-semibold text-ink">{title}</p>
        <p className="max-w-md text-sm text-ink-muted">{body}</p>
        {curl && (
          <div className="mt-2 w-full max-w-2xl overflow-x-auto rounded-lg border border-edge bg-[#04070c] p-4 text-left">
            <div className="mb-2 flex items-center justify-between gap-2">
              <span className="text-[11px] font-medium tracking-wider text-ink-faint uppercase">
                {curlLabel ?? "curl"}
              </span>
              <CopyButton text={curl} />
            </div>
            <pre className="font-mono text-xs leading-relaxed whitespace-pre text-ink-muted">
              <code>{curl}</code>
            </pre>
          </div>
        )}
        <p className="text-xs text-ink-faint">
          Guía completa paso a paso en{" "}
          <a href="/docs/quickstart" className="text-accent hover:underline">
            /docs/quickstart
          </a>
          .
        </p>
      </div>
    </Card>
  );
}
