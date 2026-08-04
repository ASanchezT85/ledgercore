import type { Metadata } from "next";
import { IdempotencyPostClient } from "./post-client";

const TITLE = "Idempotency is not a header, it's a promise";
const DESCRIPTION =
  "Accepting an Idempotency-Key and doing nothing with it is worse than not accepting one. The three guarantees it must actually make, and the race that breaks the naive implementation.";

export const metadata: Metadata = {
  title: `${TITLE} · LedgerCore`,
  description: DESCRIPTION,
  authors: [{ name: "Alexander Sanchez" }],
  openGraph: {
    type: "article",
    title: TITLE,
    description: DESCRIPTION,
    siteName: "LedgerCore",
    publishedTime: "2026-07-30T00:00:00.000Z",
    authors: ["Alexander Sanchez"],
  },
  twitter: {
    card: "summary_large_image",
    title: TITLE,
    description: DESCRIPTION,
  },
};

export default function IdempotencyPost() {
  return <IdempotencyPostClient />;
}
