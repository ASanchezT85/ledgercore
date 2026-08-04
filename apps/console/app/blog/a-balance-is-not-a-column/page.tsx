import type { Metadata } from "next";
import { BalanceColumnPostClient } from "./post-client";

const TITLE = "A balance is not a column";
const DESCRIPTION =
  "The moment you store a balance in a column you have two sources of truth and no way to tell which one is lying. Atomic materialization, the drift query, and why derived always wins.";

export const metadata: Metadata = {
  title: `${TITLE} · LedgerCore`,
  description: DESCRIPTION,
  authors: [{ name: "Alexander Sanchez" }],
  openGraph: {
    type: "article",
    title: TITLE,
    description: DESCRIPTION,
    siteName: "LedgerCore",
    publishedTime: "2026-08-01T00:00:00.000Z",
    authors: ["Alexander Sanchez"],
  },
  twitter: {
    card: "summary_large_image",
    title: TITLE,
    description: DESCRIPTION,
  },
};

export default function BalanceColumnPost() {
  return <BalanceColumnPostClient />;
}
