import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { BrandLockup } from "@/components/logo";

export const metadata: Metadata = {
  title: "Blog · LedgerCore",
  description:
    "Notes from building a double-entry ledger as a service: scars, principles, and the mechanics of correct money movement.",
  openGraph: {
    type: "website",
    title: "LedgerCore Blog",
    description:
      "Notes from building a double-entry ledger as a service: scars, principles, and the mechanics of correct money movement.",
    siteName: "LedgerCore",
  },
};

type Post = {
  slug: string;
  title: string;
  date: string;
  dateISO: string;
  excerpt: string;
};

const POSTS: Post[] = [
  {
    slug: "the-six-figure-debt",
    title: "The six-figure debt that existed in no system",
    date: "July 28, 2026",
    dateISO: "2026-07-28",
    excerpt:
      "A payment provider said we owed them six figures. Our systems said we owed them nothing. Five scars from operating money movement on mutable tables, and the ledger principles each one burned in.",
  },
];

export default function BlogIndexPage() {
  return (
    <div className="min-h-screen">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <Link href="/" aria-label="LedgerCore home">
          <BrandLockup size={30} />
        </Link>
        <nav className="flex items-center gap-2" aria-label="Main">
          <Link
            href="/"
            className="rounded-(--radius-control) px-4 py-2 text-sm text-ink-muted transition-colors hover:text-ink"
          >
            Home
          </Link>
          <Link
            href="/signup"
            className="rounded-(--radius-control) border border-edge-strong px-4 py-2 text-sm font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent"
          >
            Try the sandbox
          </Link>
        </nav>
      </header>

      <main className="mx-auto w-full max-w-3xl px-6 pt-10 pb-24">
        <h1 className="text-3xl font-semibold tracking-tight text-ink">Blog</h1>
        <p className="mt-3 text-sm leading-relaxed text-ink-muted">
          Notes from building a double-entry ledger as a service. One post, one
          technical idea.
        </p>

        <ul className="mt-10 space-y-4">
          {POSTS.map((post) => (
            <li key={post.slug}>
              <Link
                href={`/blog/${post.slug}`}
                className="group block rounded-(--radius-card) border border-edge bg-surface/70 p-6 transition-colors hover:border-accent/40"
              >
                <time
                  dateTime={post.dateISO}
                  className="text-xs tracking-wide text-ink-faint"
                >
                  {post.date}
                </time>
                <h2 className="mt-2 text-lg font-semibold text-ink transition-colors group-hover:text-accent">
                  {post.title}
                </h2>
                <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                  {post.excerpt}
                </p>
                <span className="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-accent">
                  Read the post
                  <ArrowRight size={14} aria-hidden="true" />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </main>

      <footer className="border-t border-edge">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-8">
          <BrandLockup size={22} />
          <p className="text-[11px] tracking-wide text-ink-faint">
            LedgerCore — financial infrastructure
          </p>
        </div>
      </footer>
    </div>
  );
}
