"use client";

import Link from "next/link";
import { ArrowRight, Eye } from "lucide-react";
import { LanguageProvider, useLang } from "@/components/language";
import { PublicFooter, PublicHeader } from "@/components/public-header";
import { POSTS, formatPostDate } from "@/lib/blog-posts";

function BlogIndexContent({ views }: { views: Record<string, number> }) {
  const { t, lang } = useLang();

  return (
    <div className="min-h-screen">
      <PublicHeader showHome />

      <main className="mx-auto w-full max-w-3xl px-6 pt-10 pb-24">
        <h1 className="font-serif text-[34px] leading-tight tracking-[-0.01em] text-ink">
          {t.blog.title}
        </h1>
        <p className="mt-3 text-sm leading-relaxed text-ink-muted">
          {t.blog.subtitle}
        </p>

        <ul className="mt-10 space-y-4">
          {POSTS.map((post) => {
            const count = views[post.slug] ?? 0;
            return (
              <li key={post.slug}>
                <Link
                  href={`/blog/${post.slug}`}
                  className="group block rounded-(--radius-card) border border-edge bg-surface/70 p-6 transition-colors hover:border-accent/40"
                >
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-faint">
                    <time dateTime={post.dateISO}>
                      {formatPostDate(post.dateISO, lang)}
                    </time>
                    <span aria-hidden="true">·</span>
                    <span>
                      <span className="num">{post.readingMinutes}</span>{" "}
                      {t.blog.minRead}
                    </span>
                    {count > 0 && (
                      <>
                        <span aria-hidden="true">·</span>
                        <span className="inline-flex items-center gap-1.5">
                          <Eye size={12} aria-hidden="true" />
                          <span className="num">{count.toLocaleString()}</span>
                          {count === 1 ? t.blog.viewSingular : t.blog.viewPlural}
                        </span>
                      </>
                    )}
                  </div>
                  <h2 className="mt-2 text-lg font-semibold text-ink transition-colors group-hover:text-accent">
                    {post.title[lang]}
                  </h2>
                  <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                    {post.excerpt[lang]}
                  </p>
                  <span className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-accent">
                    {t.blog.readPost}
                    <ArrowRight size={14} aria-hidden="true" />
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      </main>

      <PublicFooter />
    </div>
  );
}

export function BlogIndexClient({ views }: { views: Record<string, number> }) {
  return (
    <LanguageProvider>
      <BlogIndexContent views={views} />
    </LanguageProvider>
  );
}
