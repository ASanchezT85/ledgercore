import type { Metadata } from "next";
import { LedgersView } from "./ledgers-view";

export const metadata: Metadata = {
  title: "Ledgers",
};

export default function LedgersPage() {
  return <LedgersView />;
}
