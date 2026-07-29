import type { Metadata } from "next";
import { GuideView } from "../../docs-ui";
import { GUIDE } from "./content";

export const metadata: Metadata = {
  title: "Webhooks — LedgerCore Docs",
  description:
    "Suscripción, verificación de firma HMAC (t=/v1=) en TypeScript y PHP, ventana anti-replay de 5 minutos y rotación de secretos con 24 h de gracia.",
};

export default function Page() {
  return <GuideView content={GUIDE} />;
}
