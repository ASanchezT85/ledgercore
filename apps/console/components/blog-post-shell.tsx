"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowLeft, Eye } from "lucide-react";
import { LanguageProvider, useLang } from "@/components/language";
import { PublicFooter, PublicHeader } from "@/components/public-header";
import { BlogComments } from "@/components/blog-comments";
import { findPost, formatPostDate } from "@/lib/blog-posts";

/** Registers the view once the post is actually rendered in a browser. */
function ViewCounter({ slug }: { slug: string }) {
  const { t } = useLang();
  const [views, setViews] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch(`/api/blog/${slug}/views`, { method: "POST" });
        const data = await res.json();
        if (!cancelled && typeof data.views === "number") setViews(data.views);
      } catch {
        /* the counter is decoration — never break the post over it */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [slug]);

  if (views === null) return null;
  return (
    <span className="inline-flex items-center gap-1.5">
      <Eye size={13} aria-hidden="true" />
      <span className="num">{views.toLocaleString()}</span>
      {views === 1 ? t.blog.viewSingular : t.blog.viewPlural}
    </span>
  );
}

function Shell({
  slug,
  children,
}: {
  slug: string;
  children: React.ReactNode;
}) {
  const { t, lang } = useLang();
  const post = findPost(slug);

  return (
    <div className="min-h-screen">
      <PublicHeader showHome />

      <article className="mx-auto w-full max-w-3xl px-6 pt-6 pb-24">
        <Link
          href="/blog"
          className="inline-flex items-center gap-1.5 text-sm text-ink-muted transition-colors hover:text-ink"
        >
          <ArrowLeft size={14} aria-hidden="true" />
          {t.blog.backToBlog}
        </Link>

        {post && (
          <>
            <h1 className="mt-8 font-serif text-[34px] leading-tight tracking-[-0.01em] text-ink sm:text-[42px]">
              {post.title[lang]}
            </h1>
            <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-ink-faint">
              <time dateTime={post.dateISO}>
                {formatPostDate(post.dateISO, lang)}
              </time>
              <span>
                <span className="num">{post.readingMinutes}</span>{" "}
                {t.blog.minRead}
              </span>
              <ViewCounter slug={slug} />
            </div>
          </>
        )}

        {children}

        <BlogComments slug={slug} />
      </article>

      <PublicFooter />
    </div>
  );
}

export function BlogPostShell({
  slug,
  children,
}: {
  slug: string;
  children: React.ReactNode;
}) {
  return (
    <LanguageProvider>
      <Shell slug={slug}>{children}</Shell>
    </LanguageProvider>
  );
}

/** Renders the language-matching body of a post. */
export function PostBody({
  es,
  en,
}: {
  es: React.ReactNode;
  en: React.ReactNode;
}) {
  const { lang } = useLang();
  return <>{lang === "es" ? es : en}</>;
}
