import type { Metadata } from "next";
import { ModerationClient } from "./moderation-client";

export const metadata: Metadata = {
  title: "Comment moderation · LedgerCore",
  // Operator-only surface: keep it out of search results entirely.
  robots: { index: false, follow: false, nocache: true },
};

export default function ModerationPage() {
  return <ModerationClient />;
}
