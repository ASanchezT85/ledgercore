import type { Metadata } from "next";
import { HoldsPostClient } from "./post-client";

const TITLE = "A hold is money, not an intention";
const DESCRIPTION =
  "A wallet 97% frozen with zero open orders. Why reservations modelled as a flag strand customer funds, and what changes when a hold becomes a posting between two accounts.";

export const metadata: Metadata = {
  title: `${TITLE} · LedgerCore`,
  description: DESCRIPTION,
  authors: [{ name: "Alexander Sanchez" }],
  openGraph: {
    type: "article",
    title: TITLE,
    description: DESCRIPTION,
    siteName: "LedgerCore",
    publishedTime: "2026-08-04T00:00:00.000Z",
    authors: ["Alexander Sanchez"],
  },
  twitter: {
    card: "summary_large_image",
    title: TITLE,
    description: DESCRIPTION,
  },
};

export default function HoldsPost() {
  return <HoldsPostClient />;
}
