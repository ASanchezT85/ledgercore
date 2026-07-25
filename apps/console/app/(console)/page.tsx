import Link from "next/link";
import {
  AlertTriangle,
  ArrowLeftRight,
  Scale,
  Vault,
} from "lucide-react";
import { DemoBadge } from "@/components/demo-badge";
import { Badge, type BadgeTone } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { StatTile } from "@/components/ui/stat-tile";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { getDashboard } from "@/lib/api";
import {
  formatDateTime,
  formatInt,
  formatMoney,
  formatMoneyWithAsset,
  formatPct,
} from "@/lib/format";
import type { ReconSourceStatus, TransactionStatus } from "@/lib/types";

export const dynamic = "force-dynamic";

const TXN_STATUS_TONE: Record<TransactionStatus, BadgeTone> = {
  posted: "emerald",
  pending: "amber",
  reversed: "red",
};

const TXN_STATUS_LABEL: Record<TransactionStatus, string> = {
  posted: "posted",
  pending: "pending",
  reversed: "reversed",
};

const HEALTH_TONE: Record<ReconSourceStatus, BadgeTone> = {
  healthy: "emerald",
  degraded: "amber",
  critical: "red",
};

const HEALTH_LABEL: Record<ReconSourceStatus, string> = {
  healthy: "saludable",
  degraded: "degradado",
  critical: "crítico",
};

export default async function DashboardPage() {
  const { data, demo } = await getDashboard();
  const { stats, recentTransactions, reconHealth } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink">
            Dashboard
          </h1>
          <p className="mt-0.5 text-sm text-ink-faint">
            Estado operativo del ledger en tiempo real
          </p>
        </div>
        <DemoBadge demo={demo} />
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label="Balance custodiado"
          value={formatMoney(
            stats.custodyUnits,
            stats.custodyAsset,
            stats.custodyExponent,
          )}
          hint={`consolidado en ${stats.custodyAsset}`}
          icon={Vault}
          tone="emerald"
        />
        <StatTile
          label="Transacciones hoy"
          value={formatInt(stats.transactionsToday)}
          hint="posteadas en las últimas 24 h"
          icon={ArrowLeftRight}
          tone="sky"
        />
        <StatTile
          label="Discrepancias abiertas"
          value={formatInt(stats.openDiscrepancies)}
          hint="pendientes de investigación"
          icon={Scale}
          tone="amber"
        />
        <StatTile
          label="Proveedores descuadrados"
          value={formatInt(stats.unbalancedProviders)}
          hint="fuera de tolerancia de conciliación"
          icon={AlertTriangle}
          tone="red"
        />
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        <Card
          title="Últimas transacciones"
          subtitle="Movimientos más recientes en todos los ledgers"
          className="xl:col-span-2"
          flush
          actions={
            <Link
              href="/transactions"
              className="text-xs font-medium text-accent hover:underline"
            >
              Ver todas →
            </Link>
          }
        >
          <Table>
            <THead>
              <TH>Transacción</TH>
              <TH>Estado</TH>
              <TH className="text-right">Monto</TH>
              <TH className="text-right">Fecha</TH>
            </THead>
            <TBody>
              {recentTransactions.map((txn) => (
                <TR key={txn.id}>
                  <TD>
                    <p className="font-medium text-ink">{txn.description}</p>
                    <p className="mt-0.5 font-mono text-[11px] text-ink-faint">
                      {txn.id}
                    </p>
                  </TD>
                  <TD>
                    <Badge tone={TXN_STATUS_TONE[txn.status]} dot>
                      {TXN_STATUS_LABEL[txn.status]}
                    </Badge>
                  </TD>
                  <TD className="text-right">
                    <span className="num font-medium text-ink">
                      {formatMoneyWithAsset(
                        txn.amountUnits,
                        txn.asset,
                        txn.exponent,
                      )}
                    </span>
                  </TD>
                  <TD className="text-right text-xs whitespace-nowrap">
                    {formatDateTime(txn.createdAt)}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Card>

        <Card
          title="Salud de conciliación"
          subtitle="Cobertura de matching por fuente externa"
        >
          <ul className="space-y-4">
            {reconHealth.map((source) => (
              <li key={source.source}>
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-ink">
                      {source.label}
                    </p>
                    <p className="font-mono text-[11px] text-ink-faint">
                      {source.source}
                    </p>
                  </div>
                  <Badge tone={HEALTH_TONE[source.status]} dot>
                    {HEALTH_LABEL[source.status]}
                  </Badge>
                </div>
                <div className="mt-2 flex items-center gap-3">
                  <div
                    className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-raised"
                    role="progressbar"
                    aria-valuenow={source.matchedPct}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-label={`Matching ${source.label}`}
                  >
                    <div
                      className={`h-full rounded-full ${
                        source.status === "critical"
                          ? "bg-danger"
                          : source.status === "degraded"
                            ? "bg-warning"
                            : "bg-accent"
                      }`}
                      style={{ width: `${source.matchedPct}%` }}
                    />
                  </div>
                  <span className="num w-12 text-right text-xs font-medium text-ink">
                    {formatPct(source.matchedPct)}
                  </span>
                </div>
                <p className="mt-1.5 text-[11px] text-ink-faint">
                  {source.openDiscrepancies} discrepancia(s) abierta(s) · última
                  corrida {formatDateTime(source.lastRunAt)}
                </p>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </div>
  );
}
