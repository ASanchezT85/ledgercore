import type { Metadata } from "next";
import { GuideView } from "../../docs-ui";
import { GUIDE } from "./content";

export const metadata: Metadata = {
  title: "Idempotencia — LedgerCore Docs",
  description:
    "idempotency_key, la cabecera X-Idempotent-Replay y la semántica exacta de los reintentos: por qué un retry nunca duplica dinero.",
};

export default function Page() {
  return <GuideView content={GUIDE} />;
}
