import type { Metadata } from "next";
import { DocsIndexView } from "./docs-index-view";

export const metadata: Metadata = {
  title: "Documentación para developers — LedgerCore",
  description:
    "Quickstart, referencia interactiva de la API (OpenAPI), guías de integración (autenticación, dinero, idempotencia, paginación, webhooks, errores) y SDKs oficiales de TypeScript y PHP.",
};

export default function DocsIndexPage() {
  return <DocsIndexView />;
}
