"use client";

import { useCallback, useEffect, useState } from "react";
import { CornerDownRight, MessageSquare, Send } from "lucide-react";
import { useLang } from "@/components/language";

type Comment = {
  id: string;
  parentId: string | null;
  authorName: string;
  body: string;
  createdAt: string;
};

type Thread = Comment & { replies: Comment[] };

function initials(name: string): string {
  return name
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase() ?? "")
    .join("");
}

function CommentForm({
  slug,
  parentId,
  onPosted,
  onCancel,
  compact = false,
}: {
  slug: string;
  parentId: string | null;
  onPosted: () => void;
  onCancel?: () => void;
  compact?: boolean;
}) {
  const { t } = useLang();
  const c = t.comments;
  const [name, setName] = useState("");
  const [body, setBody] = useState("");
  const [website, setWebsite] = useState(""); // honeypot
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy || name.trim().length < 2 || body.trim().length < 2) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/blog/${slug}/comments`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          author_name: name,
          body,
          parent_id: parentId,
          website,
        }),
      });
      if (res.status === 429) {
        setError(c.errRateLimited);
      } else if (res.status === 503) {
        setError(c.errUnavailable);
      } else if (!res.ok) {
        setError(c.errInvalid);
      } else {
        setName("");
        setBody("");
        onPosted();
        return;
      }
    } catch {
      setError(c.errNetwork);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className={compact ? "mt-3" : "mt-6"}>
      <div className="space-y-3 rounded-(--radius-card) border border-edge bg-surface/70 p-4">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={c.namePlaceholder}
          maxLength={60}
          aria-label={c.nameLabel}
          className="w-full rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2 text-sm text-ink outline-none placeholder:text-ink-faint focus:border-accent/60"
        />
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder={parentId ? c.replyPlaceholder : c.bodyPlaceholder}
          rows={compact ? 3 : 4}
          maxLength={4000}
          aria-label={c.bodyLabel}
          className="w-full resize-y rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2 text-sm leading-relaxed text-ink outline-none placeholder:text-ink-faint focus:border-accent/60"
        />

        {/* Honeypot: hidden from humans and from screen readers. */}
        <input
          type="text"
          value={website}
          onChange={(e) => setWebsite(e.target.value)}
          name="website"
          tabIndex={-1}
          autoComplete="off"
          aria-hidden="true"
          className="hidden"
        />

        {error && (
          <p role="alert" className="text-xs text-danger">
            {error}
          </p>
        )}

        <div className="flex items-center justify-between gap-3">
          <p className="text-[11px] text-ink-faint">{c.footnote}</p>
          <div className="flex items-center gap-2">
            {onCancel && (
              <button
                type="button"
                onClick={onCancel}
                className="rounded-(--radius-control) px-3 py-1.5 text-xs text-ink-muted transition-colors hover:text-ink"
              >
                {c.cancel}
              </button>
            )}
            <button
              type="submit"
              disabled={busy || name.trim().length < 2 || body.trim().length < 2}
              className="inline-flex items-center gap-1.5 rounded-(--radius-control) bg-accent-deep px-4 py-2 text-xs font-semibold text-[#03150e] transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Send size={13} aria-hidden="true" />
              {busy ? c.sending : parentId ? c.sendReply : c.send}
            </button>
          </div>
        </div>
      </div>
    </form>
  );
}

function CommentCard({
  comment,
  slug,
  onPosted,
  isReply = false,
}: {
  comment: Comment;
  slug: string;
  onPosted: () => void;
  isReply?: boolean;
}) {
  const { t, lang } = useLang();
  const c = t.comments;
  const [replying, setReplying] = useState(false);

  const when = new Date(comment.createdAt).toLocaleDateString(
    lang === "es" ? "es-ES" : "en-US",
    { year: "numeric", month: "short", day: "numeric" },
  );

  return (
    <div
      className={
        isReply
          ? "border-l border-edge pl-4"
          : "rounded-(--radius-card) border border-edge bg-surface/50 p-5"
      }
    >
      <div className="flex items-center gap-2.5">
        <span className="inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[10px] font-semibold text-accent">
          {initials(comment.authorName)}
        </span>
        <span className="text-sm font-medium text-ink">{comment.authorName}</span>
        <time
          dateTime={comment.createdAt}
          className="text-[11px] text-ink-faint"
        >
          {when}
        </time>
      </div>

      <p className="mt-2.5 text-sm leading-relaxed whitespace-pre-wrap text-ink-muted">
        {comment.body}
      </p>

      {!isReply && (
        <>
          <button
            type="button"
            onClick={() => setReplying((v) => !v)}
            className="mt-3 inline-flex items-center gap-1.5 text-xs font-medium text-accent transition-opacity hover:opacity-80"
          >
            <CornerDownRight size={13} aria-hidden="true" />
            {replying ? c.cancelReply : c.reply}
          </button>
          {replying && (
            <CommentForm
              slug={slug}
              parentId={comment.id}
              compact
              onCancel={() => setReplying(false)}
              onPosted={() => {
                setReplying(false);
                onPosted();
              }}
            />
          )}
        </>
      )}
    </div>
  );
}

export function BlogComments({ slug }: { slug: string }) {
  const { t } = useLang();
  const c = t.comments;
  const [threads, setThreads] = useState<Thread[] | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await fetch(`/api/blog/${slug}/comments`, { cache: "no-store" });
      const data = await res.json();
      setThreads(Array.isArray(data.comments) ? data.comments : []);
    } catch {
      setThreads([]);
    }
  }, [slug]);

  useEffect(() => {
    void load();
  }, [load]);

  const total =
    threads?.reduce((n, th) => n + 1 + th.replies.length, 0) ?? 0;

  return (
    <section className="mt-16 border-t border-edge pt-10">
      <h2 className="inline-flex items-center gap-2 text-lg font-semibold text-ink">
        <MessageSquare size={17} className="text-accent" aria-hidden="true" />
        {c.title}
        {threads !== null && total > 0 && (
          <span className="num text-sm font-normal text-ink-faint">({total})</span>
        )}
      </h2>

      <CommentForm slug={slug} parentId={null} onPosted={load} />

      {threads !== null && threads.length === 0 && (
        <p className="mt-8 text-sm text-ink-faint">{c.empty}</p>
      )}

      <div className="mt-8 space-y-4">
        {threads?.map((thread) => (
          <div key={thread.id}>
            <CommentCard comment={thread} slug={slug} onPosted={load} />
            {thread.replies.length > 0 && (
              <div className="mt-3 ml-5 space-y-4">
                {thread.replies.map((reply) => (
                  <CommentCard
                    key={reply.id}
                    comment={reply}
                    slug={slug}
                    onPosted={load}
                    isReply
                  />
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
