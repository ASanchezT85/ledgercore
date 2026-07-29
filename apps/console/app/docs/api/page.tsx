import type { Metadata } from "next";
import { ApiReferenceView } from "./api-view";

export const metadata: Metadata = {
  title: "Referencia de API — LedgerCore",
  description:
    "Referencia interactiva de los cuatro contratos OpenAPI de LedgerCore: Identity, Ledger Core, Reconciliation y Webhooks — la fuente de verdad de la API.",
};

export default function ApiReferencePage() {
  return <ApiReferenceView />;
}
