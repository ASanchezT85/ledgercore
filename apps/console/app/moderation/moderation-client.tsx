"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  Eye,
  EyeOff,
  ExternalLink,
  KeyRound,
  LogOut,
  Trash2,
} from "lucide-react";
import { LanguageProvider, useLang } from "@/components/language";
import { PublicFooter, PublicHeader } from "@/components/public-header";

const TOKEN_KEY = "lc_blog_admin_token";

type ModeratedComment = {
  id: string;
  slug: string;
  parentId: string | null;
  parentAuthor: string | null;
  authorName: string;
  body: string;
  status: "visible" | "hidden";
  createdAt: string;
};

type Filter = "all" | "visible" | "hidden";

function TokenGate({ onAuthed }: { onAuthed: (token: string) => void }) {
  const { t } = useLang();
  const c = t.moderation;
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!token.trim() || busy) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/blog/admin/comments", {
        headers: { Authorization: `Bearer ${token.trim()}` },
        cache: "no-store",
      });
      if (res.status === 401) setError(c.errUnauthorized);
      else if (res.status === 503) setError(c.errNotConfigured);
      else if (!res.ok) setError(c.errUnavailable);
      else {
        try {
          window.sessionStorage.setItem(TOKEN_KEY, token.trim());
        } catch {
          /* sessionStorage bloqueado: la sesión vive solo en memoria */
        }
        onAuthed(token.trim());
        return;
      }
    } catch {
      setError(c.errUnavailable);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="mx-auto mt-16 w-full max-w-sm">
      <div className="rounded-(--radius-card) border border-edge bg-surface/80 p-6">
        <h1 className="text-sm font-semibold text-ink">{c.title}</h1>
        <label className="mt-5 block">
          <span className="mb-1.5 block text-xs font-medium text-ink-muted">
            {c.tokenLabel}
          </span>
          <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5 focus-within:border-accent/60">
            <KeyRound size={14} className="shrink-0 text-ink-faint" aria-hidden="true" />
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={c.tokenPlaceholder}
              autoComplete="off"
              spellCheck={false}
              aria-label={c.tokenLabel}
              className="w-full bg-transparent font-mono text-sm text-ink outline-none placeholder:font-sans placeholder:text-ink-faint"
            />
          </span>
        </label>

        {error && (
          <p role="alert" className="mt-3 text-xs text-danger">
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={busy || !token.trim()}
          className="mt-4 inline-flex h-10 w-full items-center justify-center rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
        >
          {c.enter}
        </button>
        <p className="mt-3 text-[11px] leading-relaxed text-ink-faint">
          {c.tokenHint}
        </p>
      </div>
    </form>
  );
}

function CommentRow({
  comment,
  token,
  onChanged,
}: {
  comment: ModeratedComment;
  token: string;
  onChanged: () => void;
}) {
  const { t, lang } = useLang();
  const c = t.moderation;
  const [busy, setBusy] = useState(false);

  const when = new Date(comment.createdAt).toLocaleString(
    lang === "es" ? "es-ES" : "en-US",
    { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" },
  );

  async function act(method: "PATCH" | "DELETE", body: object) {
    setBusy(true);
    try {
      await fetch("/api/blog/admin/comments", {
        method,
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(body),
      });
      onChanged();
    } finally {
      setBusy(false);
    }
  }

  const hidden = comment.status === "hidden";

  return (
    <li
      className={
        "rounded-(--radius-card) border p-5 transition-colors " +
        (hidden
          ? "border-edge bg-surface/30 opacity-70"
          : "border-edge bg-surface/70")
      }
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <span className="text-sm font-medium text-ink">{comment.authorName}</span>
        <span
          className={
            "rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wide uppercase " +
            (hidden
              ? "bg-surface-raised text-ink-faint"
              : "bg-accent-soft text-accent")
          }
        >
          {hidden ? c.statusHidden : c.statusVisible}
        </span>
        {comment.parentAuthor && (
          <span className="text-[11px] text-ink-faint">
            {c.replyTo} {comment.parentAuthor}
          </span>
        )}
        <time dateTime={comment.createdAt} className="text-[11px] text-ink-faint">
          {when}
        </time>
      </div>

      <p className="mt-2.5 text-sm leading-relaxed whitespace-pre-wrap text-ink-muted">
        {comment.body}
      </p>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={() =>
            act("PATCH", {
              id: comment.id,
              status: hidden ? "visible" : "hidden",
            })
          }
          className="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-edge-strong px-3 py-1.5 text-xs font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent disabled:opacity-50"
        >
          {hidden ? <Eye size={13} aria-hidden="true" /> : <EyeOff size={13} aria-hidden="true" />}
          {hidden ? c.restore : c.hide}
        </button>

        <button
          type="button"
          disabled={busy}
          onClick={() => {
            if (window.confirm(c.confirmRemove)) act("DELETE", { id: comment.id });
          }}
          className="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-danger/40 px-3 py-1.5 text-xs font-medium text-danger transition-colors hover:bg-danger/10 disabled:opacity-50"
        >
          <Trash2 size={13} aria-hidden="true" />
          {c.remove}
        </button>

        <Link
          href={`/blog/${comment.slug}`}
          className="inline-flex items-center gap-1.5 px-2 py-1.5 text-xs text-ink-faint transition-colors hover:text-ink"
        >
          <ExternalLink size={12} aria-hidden="true" />
          <span className="font-mono">{comment.slug}</span>
        </Link>
      </div>
    </li>
  );
}

function ModerationContent() {
  const { t } = useLang();
  const c = t.moderation;
  const [token, setToken] = useState<string | null>(null);
  const [ready, setReady] = useState(false);
  const [comments, setComments] = useState<ModeratedComment[] | null>(null);
  const [filter, setFilter] = useState<Filter>("all");

  useEffect(() => {
    try {
      setToken(window.sessionStorage.getItem(TOKEN_KEY));
    } catch {
      /* sessionStorage bloqueado */
    }
    setReady(true);
  }, []);

  const load = useCallback(async (tk: string) => {
    try {
      const res = await fetch("/api/blog/admin/comments", {
        headers: { Authorization: `Bearer ${tk}` },
        cache: "no-store",
      });
      if (res.status === 401) {
        // Token revoked or rotated on the server — back to the gate.
        try {
          window.sessionStorage.removeItem(TOKEN_KEY);
        } catch {
          /* noop */
        }
        setToken(null);
        return;
      }
      const data = await res.json();
      setComments(Array.isArray(data.comments) ? data.comments : []);
    } catch {
      setComments([]);
    }
  }, []);

  useEffect(() => {
    if (token) void load(token);
  }, [token, load]);

  if (!ready) return null;

  if (!token) {
    return <TokenGate onAuthed={setToken} />;
  }

  const shown = (comments ?? []).filter((x) =>
    filter === "all" ? true : x.status === filter,
  );

  return (
    <main className="mx-auto w-full max-w-3xl px-6 pt-6 pb-24">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="font-serif text-[28px] leading-tight tracking-[-0.01em] text-ink">
            {c.title}
          </h1>
          <p className="mt-2 max-w-xl text-sm leading-relaxed text-ink-muted">
            {c.subtitle}
          </p>
        </div>
        <button
          type="button"
          onClick={() => {
            try {
              window.sessionStorage.removeItem(TOKEN_KEY);
            } catch {
              /* noop */
            }
            setToken(null);
            setComments(null);
          }}
          className="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-edge-strong px-3 py-1.5 text-xs text-ink-muted transition-colors hover:text-ink"
        >
          <LogOut size={13} aria-hidden="true" />
          {c.signOut}
        </button>
      </div>

      <div className="mt-6 flex flex-wrap items-center gap-2">
        {(["all", "visible", "hidden"] as const).map((f) => (
          <button
            key={f}
            type="button"
            onClick={() => setFilter(f)}
            aria-pressed={filter === f}
            className={
              "rounded-(--radius-control) px-3 py-1.5 text-xs font-medium transition-colors " +
              (filter === f
                ? "bg-accent-soft text-accent"
                : "text-ink-faint hover:text-ink")
            }
          >
            {f === "all" ? c.filterAll : f === "visible" ? c.filterVisible : c.filterHidden}
          </button>
        ))}
        {comments !== null && (
          <span className="ml-auto text-xs text-ink-faint">
            <span className="num">{shown.length}</span> {c.total}
          </span>
        )}
      </div>

      {comments !== null && shown.length === 0 && (
        <p className="mt-10 text-sm text-ink-faint">{c.empty}</p>
      )}

      <ul className="mt-6 space-y-3">
        {shown.map((comment) => (
          <CommentRow
            key={comment.id}
            comment={comment}
            token={token}
            onChanged={() => load(token)}
          />
        ))}
      </ul>
    </main>
  );
}

export function ModerationClient() {
  return (
    <LanguageProvider>
      <div className="flex min-h-screen flex-col">
        <PublicHeader showHome />
        <div className="flex-1">
          <ModerationContent />
        </div>
        <PublicFooter />
      </div>
    </LanguageProvider>
  );
}
