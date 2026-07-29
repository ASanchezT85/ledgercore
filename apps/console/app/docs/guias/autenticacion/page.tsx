import type { Metadata } from "next";
import { GuideView } from "../../docs-ui";
import { GUIDE } from "./content";

export const metadata: Metadata = {
  title: "Autenticación — LedgerCore Docs",
  description:
    "API key → token exchange → JWT EdDSA de 15 minutos. Cómo autenticarse contra la API de LedgerCore con curl, TypeScript y PHP.",
};

export default function Page() {
  return <GuideView content={GUIDE} />;
}
