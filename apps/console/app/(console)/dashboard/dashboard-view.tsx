"use client";

import Link from "next/link";
import {
  AlertTriangle,
  ArrowLeftRight,
  BookOpenText,
  Scale,
  Vault,
} from "lucide-react";
import { DemoBadge } from "@/components/demo-badge";
import {
  ApiDownBanner,
  EmptyState,
  ErrorBanner,
  LoadingCard,
  useLiveData,
} from "@/components/live";
import { Badge, type BadgeTone } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { StatTile } from "@/components/ui/stat-tile";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import {
  formatDateTime,
  formatInt,
  formatMoney,
  formatMoneyWithAsset,
  formatPct,
} from "@/lib/format";
import {
  assetExponent,
  loadDashboard,
  transactionTotals,
  type ApiTxnStatus,
  type LiveDashboard,
} from "@/lib/live";
import { mockDashboard } from "@/lib/mock-data";
import type { ReconSourceStatus, TransactionStatus } from "@/lib/types";

const TXN_STATUS_TONE: Record<TransactionStatus | ApiTxnStatus, BadgeTone> = {
  posted: "emerald",
  pending: "amber",
  draft: "amber",
  reversed: "red",
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

const CREATE_LEDGER_CURL = `curl -s -X POST $API/v1/ledgers \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{ "name": "main", "description": "Primary operating ledger" }'`;

function Header({ demo }: { demo: boolean }) {
  return (
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
  );
}

// ---- Live dashboard -------------------------------------------------------

function LiveDashboardView({ data }: { data: LiveDashboard }) {
  if (data.ledgers.length === 0) {
    return (
      <EmptyState
        title="Tu tenant todavía no tiene ledgers"
        body="Crea tu primer ledger con la API y este dashboard cobrará vida: balances por asset, transacciones recientes y el trial balance verificado."
        curl={CREATE_LEDGER_CURL}
        curlLabel="POST /v1/ledgers"
      />
    );
  }

  const custodyMain = data.custody[0] ?? null;
  const balancedTotals = data.trial?.totals ?? [];
  const allBalanced =
    balancedTotals.length > 0 && balancedTotals.every((t) => t.balanced);

  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label="Balance custodiado"
          value={
            custodyMain
              ? formatMoney(custodyMain.units, custodyMain.asset, custodyMain.exponent)
              : "—"
          }
          hint={
            data.custody.length > 1
              ? `+ ${data.custody.length - 1} asset(s) más`
              : custodyMain
                ? `neto posteado en cuentas asset (${custodyMain.asset})`
                : "sin balances aún"
          }
          icon={Vault}
          tone="emerald"
        />
        <StatTile
          label="Transacciones (24 h)"
          value={formatInt(data.transactionsToday)}
          hint={`${formatInt(data.transactions.length)} recientes cargadas`}
          icon={ArrowLeftRight}
          tone="sky"
        />
        <StatTile
          label="Ledgers · cuentas"
          value={`${formatInt(data.ledgers.length)} · ${formatInt(data.accountCount)}`}
          hint="libros y cuentas activas del tenant"
          icon={BookOpenText}
          tone="amber"
        />
        <StatTile
          label="Trial balance"
          value={
            data.trial === null ? "—" : allBalanced ? "balanceado" : "descuadrado"
          }
          hint={
            data.trial === null
              ? "sin transacciones posteadas"
              : `ledger ${data.ledgers[0]?.name ?? ""} · débitos = créditos`
          }
          icon={allBalanced || data.trial === null ? Scale : AlertTriangle}
          tone={data.trial === null ? "sky" : allBalanced ? "emerald" : "red"}
        />
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
        <Card
          title="Últimas transacciones"
          subtitle="Movimientos más recientes del tenant"
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
          {data.transactions.length === 0 ? (
            <p className="px-5 py-10 text-center text-sm text-ink-faint">
              Aún no hay transacciones. Postea la primera desde el{" "}
              <Link href="/docs/quickstart" className="text-accent hover:underline">
                quickstart
              </Link>
              .
            </p>
          ) : (
            <Table>
              <THead>
                <TH>Transacción</TH>
                <TH>Estado</TH>
                <TH className="text-right">Monto</TH>
                <TH className="text-right">Fecha</TH>
              </THead>
              <TBody>
                {data.transactions.map((txn) => {
                  const main = transactionTotals(txn)[0] ?? null;
                  return (
                    <TR key={txn.id}>
                      <TD>
                        <p className="font-medium text-ink">
                          {txn.description || txn.idempotency_key}
                        </p>
                        <p className="mt-0.5 font-mono text-[11px] text-ink-faint">
                          {txn.id}
                        </p>
                      </TD>
                      <TD>
                        <Badge tone={TXN_STATUS_TONE[txn.status]} dot>
                          {txn.status}
                        </Badge>
                      </TD>
                      <TD className="text-right">
                        <span className="num font-medium text-ink">
                          {main
                            ? formatMoneyWithAsset(
                                BigInt(main.amount),
                                main.asset,
                                assetExponent(main.asset),
                              )
                            : "—"}
                        </span>
                      </TD>
                      <TD className="text-right text-xs whitespace-nowrap">
                        {formatDateTime(txn.created_at)}
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          )}
        </Card>

        <Card
          title="Trial balance"
          subtitle={
            data.trial
              ? `Ledger ${data.ledgers[0]?.name ?? ""} · débitos vs créditos por asset`
              : "Se calcula sobre transacciones posteadas"
          }
        >
          {data.trial === null || balancedTotals.length === 0 ? (
            <p className="py-6 text-center text-sm text-ink-faint">
              Sin datos aún: postea una transacción y el reporte aparecerá aquí.
            </p>
          ) : (
            <ul className="space-y-4">
              {balancedTotals.map((total) => (
                <li key={total.asset}>
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-sm font-medium text-ink">{total.asset}</p>
                    <Badge tone={total.balanced ? "emerald" : "red"} dot>
                      {total.balanced ? "balanceado" : "descuadrado"}
                    </Badge>
                  </div>
                  <div className="mt-2 grid grid-cols-2 gap-3">
                    <div className="rounded-lg border border-edge bg-surface-raised px-3 py-2">
                      <p className="text-[10px] font-semibold tracking-wider text-ink-faint uppercase">
                        Débitos
                      </p>
                      <p className="num mt-0.5 text-sm font-medium text-ink">
                        {formatMoney(
                          BigInt(total.debits),
                          total.asset,
                          assetExponent(total.asset),
                        )}
                      </p>
                    </div>
                    <div className="rounded-lg border border-edge bg-surface-raised px-3 py-2">
                      <p className="text-[10px] font-semibold tracking-wider text-ink-faint uppercase">
                        Créditos
                      </p>
                      <p className="num mt-0.5 text-sm font-medium text-ink">
                        {formatMoney(
                          BigInt(total.credits),
                          total.asset,
                          assetExponent(total.asset),
                        )}
                      </p>
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </>
  );
}

// ---- Demo dashboard (unchanged visual, mock dataset) ----------------------

function DemoDashboardView() {
  const { stats, recentTransactions, reconHealth } = mockDashboard;
  return (
    <>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatTile
          label="Balance custodiado"
          value={formatMoney(stats.custodyUnits, stats.custodyAsset, stats.custodyExponent)}
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
                      {txn.status}
                    </Badge>
                  </TD>
                  <TD className="text-right">
                    <span className="num font-medium text-ink">
                      {formatMoneyWithAsset(txn.amountUnits, txn.asset, txn.exponent)}
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
    </>
  );
}

export function DashboardView() {
  const state = useLiveData(loadDashboard);

  return (
    <div className="space-y-6">
      <Header demo={state.status === "demo"} />
      {state.status === "loading" && <LoadingCard />}
      {state.status === "demo" && <DemoDashboardView />}
      {state.status === "down" && <ApiDownBanner onRetry={state.reload} />}
      {state.status === "error" && (
        <ErrorBanner message={state.message} onRetry={state.reload} />
      )}
      {state.status === "live" && <LiveDashboardView data={state.data} />}
    </div>
  );
}
