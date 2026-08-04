import type { Metadata } from "next";
import { BlogIndexClient } from "./blog-index-client";
import { POSTS } from "@/lib/blog-posts";
import { viewCounts } from "@/lib/blog-db";

export const dynamic = "force-dynamic";

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

export default async function BlogIndexPage() {
  // A database hiccup must not take the blog down: fall back to no counters.
  const views = await viewCounts(POSTS.map((p) => p.slug)).catch(() => ({}));
  return <BlogIndexClient views={views} />;
}
