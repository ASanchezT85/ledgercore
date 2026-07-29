import type { Metadata } from "next";
import { GuideView } from "../../docs-ui";
import { GUIDE } from "./content";

export const metadata: Metadata = {
  title: "Dinero y montos — LedgerCore Docs",
  description:
    "Enteros en unidades menores, strings en JSON, Money helpers en TypeScript y PHP — y por qué un ledger jamás usa floats.",
};

export default function Page() {
  return <GuideView content={GUIDE} />;
}
