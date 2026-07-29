import type { Metadata } from "next";
import { GuideView } from "../../docs-ui";
import { GUIDE } from "./content";

export const metadata: Metadata = {
  title: "Paginación — LedgerCore Docs",
  description:
    "limit / cursor / next_cursor: el contrato uniforme de paginación keyset de la API (default 50, máximo 200) y la autopaginación de los SDKs.",
};

export default function Page() {
  return <GuideView content={GUIDE} />;
}
