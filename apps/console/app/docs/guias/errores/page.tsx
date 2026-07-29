import type { Metadata } from "next";
import { GuideView } from "../../docs-ui";
import { GUIDE } from "./content";

export const metadata: Metadata = {
  title: "Errores — LedgerCore Docs",
  description:
    "El catálogo estable de errores de la API: formato único {code, message, request_id} y tabla completa código → HTTP → cuándo ocurre → qué hacer.",
};

export default function Page() {
  return <GuideView content={GUIDE} />;
}
