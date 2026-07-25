import type { Metadata } from "next";
import { KeyRound, Plus, Terminal, Webhook } from "lucide-react";
import { DemoBadge } from "@/components/demo-badge";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { getDeveloperData } from "@/lib/api";
import { formatDate, formatDateTime } from "@/lib/format";

export const metadata: Metadata = {
  title: "Developers",
};

export const dynamic = "force-dynamic";

const CURL_SNIPPET = `curl -X POST http://localhost:8080/v1/transactions \\
  -H "Authorization: Bearer $LEDGERCORE_API_KEY" \\
  -H "Idempotency-Key: dep_9f2c41d7" \\
  -H "Content-Type: application/json" \\
  -d '{
    "ledger": "main",
    "description": "Customer deposit — USD 100",
    "postings": [
      { "account": "assets/custody/bank/bofa",              "direction": "debit",  "amount": 10000, "asset": "USD" },
      { "account": "liabilities/customers/cus_8c31/wallet", "direction": "credit", "amount": 9700,  "asset": "USD" },
      { "account": "revenue/fees/deposit",                  "direction": "credit", "amount": 300,   "asset": "USD" }
    ]
  }'`;

export default async function DevelopersPage() {
  const { data, demo } = await getDeveloperData();
  const { apiKeys, webhooks } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink">
            Developers
          </h1>
          <p className="mt-0.5 text-sm text-ink-faint">
            Credenciales, webhooks y primeros pasos para integrar LedgerCore
          </p>
        </div>
        <DemoBadge demo={demo} />
      </div>

      <Card
        title="API keys"
        subtitle="Las llaves se muestran una sola vez al crearlas; aquí solo el prefijo"
        flush
        actions={
          <span className="group relative inline-block">
            <button
              type="button"
              disabled
              className="inline-flex h-8 cursor-not-allowed items-center gap-1.5 rounded-(--radius-control) border border-edge-strong px-3 text-xs font-medium text-ink-faint opacity-60"
            >
              <Plus size={13} aria-hidden="true" />
              Crear llave
            </button>
            <span
              role="tooltip"
              className="pointer-events-none absolute top-full right-0 z-10 mt-2 w-56 rounded-lg border border-edge bg-surface-raised px-3 py-2 text-[11px] text-ink-muted opacity-0 shadow-xl transition-opacity duration-150 group-hover:opacity-100"
            >
              La emisión de llaves se habilita cuando el servicio de identity
              esté en línea.
            </span>
          </span>
        }
      >
        <Table>
          <THead>
            <TH>Nombre</TH>
            <TH>Prefijo</TH>
            <TH>Creada</TH>
            <TH className="text-right">Último uso</TH>
          </THead>
          <TBody>
            {apiKeys.map((apiKey) => (
              <TR key={apiKey.id}>
                <TD>
                  <span className="inline-flex items-center gap-2 font-medium text-ink">
                    <KeyRound size={13} className="text-ink-faint" aria-hidden="true" />
                    {apiKey.name}
                  </span>
                </TD>
                <TD>
                  <code className="rounded-md border border-edge bg-surface-raised px-2 py-0.5 font-mono text-xs text-accent">
                    {apiKey.prefix}
                  </code>
                </TD>
                <TD className="text-xs">{formatDate(apiKey.createdAt)}</TD>
                <TD className="text-right text-xs whitespace-nowrap">
                  {apiKey.lastUsedAt ? (
                    formatDateTime(apiKey.lastUsedAt)
                  ) : (
                    <span className="text-ink-faint">nunca</span>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Card>

      <Card
        title="Webhook subscriptions"
        subtitle="Entrega firmada de eventos del envelope v1 vía NATS → webhooks"
        flush
      >
        <Table>
          <THead>
            <TH>Endpoint</TH>
            <TH>Topics</TH>
            <TH>Estado</TH>
            <TH className="text-right">Creada</TH>
          </THead>
          <TBody>
            {webhooks.map((subscription) => (
              <TR key={subscription.id}>
                <TD>
                  <span className="inline-flex items-center gap-2">
                    <Webhook size={13} className="text-ink-faint" aria-hidden="true" />
                    <span className="font-mono text-xs text-ink">
                      {subscription.url}
                    </span>
                  </span>
                </TD>
                <TD>
                  <div className="flex flex-wrap gap-1.5">
                    {subscription.topics.map((topic) => (
                      <Badge key={topic} tone="slate" className="font-mono">
                        {topic}
                      </Badge>
                    ))}
                  </div>
                </TD>
                <TD>
                  <Badge
                    tone={subscription.status === "active" ? "emerald" : "amber"}
                    dot
                  >
                    {subscription.status === "active" ? "activa" : "pausada"}
                  </Badge>
                </TD>
                <TD className="text-right text-xs whitespace-nowrap">
                  {formatDate(subscription.createdAt)}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Card>

      <Card
        title="Primeros pasos"
        subtitle="Postea tu primera transacción de doble partida contra el gateway"
      >
        <div className="space-y-3">
          <p className="text-sm text-ink-muted">
            El ejemplo canónico: un depósito de{" "}
            <span className="num font-medium text-ink">$100.00 USD</span> que
            acredita{" "}
            <span className="num font-medium text-accent">$97.00</span> al
            wallet del cliente y{" "}
            <span className="num font-medium text-accent">$3.00</span> a
            ingresos por fee. Los montos van en{" "}
            <strong className="font-medium text-ink">
              unidades mínimas enteras
            </strong>{" "}
            (10000 = $100.00) — nunca floats.
          </p>
          <div className="overflow-x-auto rounded-lg border border-edge bg-[#04070c] p-4">
            <div className="mb-3 flex items-center gap-2 text-ink-faint">
              <Terminal size={13} aria-hidden="true" />
              <span className="text-[11px] font-medium tracking-wider uppercase">
                POST /v1/transactions
              </span>
            </div>
            <pre className="font-mono text-xs leading-relaxed text-ink-muted">
              <code>{CURL_SNIPPET}</code>
            </pre>
          </div>
          <p className="text-xs text-ink-faint">
            La cabecera{" "}
            <code className="font-mono text-ink-muted">Idempotency-Key</code>{" "}
            garantiza exactamente-una-vez: reintentos con la misma llave
            devuelven la transacción original.
          </p>
        </div>
      </Card>
    </div>
  );
}
