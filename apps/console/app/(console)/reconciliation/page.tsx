import type { Metadata } from "next";
import { DemoBadge } from "@/components/demo-badge";
import { Badge, type BadgeTone } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { getReconciliation } from "@/lib/api";
import {
  formatDateTime,
  formatInt,
  formatMoneyWithAsset,
} from "@/lib/format";
import type {
  DiscrepancyStatus,
  DiscrepancyType,
  ReconRunStatus,
} from "@/lib/types";

export const metadata: Metadata = {
  title: "Conciliación",
};

export const dynamic = "force-dynamic";

const RUN_TONE: Record<ReconRunStatus, BadgeTone> = {
  completed: "emerald",
  running: "sky",
  failed: "red",
};

const RUN_LABEL: Record<ReconRunStatus, string> = {
  completed: "completada",
  running: "en curso",
  failed: "fallida",
};

const TYPE_TONE: Record<DiscrepancyType, BadgeTone> = {
  missing_internal: "red",
  missing_external: "amber",
  amount_mismatch: "violet",
  duplicate: "sky",
};

const STATUS_TONE: Record<DiscrepancyStatus, BadgeTone> = {
  open: "red",
  investigating: "amber",
  resolved: "emerald",
};

const STATUS_LABEL: Record<DiscrepancyStatus, string> = {
  open: "open",
  investigating: "investigating",
  resolved: "resolved",
};

export default async function ReconciliationPage() {
  const { data, demo } = await getReconciliation();
  const { runs, discrepancies } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink">
            Conciliación
          </h1>
          <p className="mt-0.5 text-sm text-ink-faint">
            Matching automático entre el ledger interno y las fuentes externas
            (bancos, PSPs, proveedores)
          </p>
        </div>
        <DemoBadge demo={demo} />
      </div>

      <Card
        title="Corridas recientes"
        subtitle="Ventanas de conciliación ejecutadas por el servicio de recon"
        flush
      >
        <Table>
          <THead>
            <TH>Run</TH>
            <TH>Fuente</TH>
            <TH>Ventana</TH>
            <TH>Estado</TH>
            <TH className="text-right">Matched</TH>
            <TH className="text-right">Unmatched</TH>
          </THead>
          <TBody>
            {runs.map((run) => (
              <TR key={run.id}>
                <TD>
                  <span className="font-mono text-xs text-accent">{run.id}</span>
                </TD>
                <TD>
                  <span className="font-mono text-xs">{run.source}</span>
                </TD>
                <TD className="text-xs whitespace-nowrap">
                  {formatDateTime(run.windowStart)} —{" "}
                  {formatDateTime(run.windowEnd)}
                </TD>
                <TD>
                  <Badge tone={RUN_TONE[run.status]} dot>
                    {RUN_LABEL[run.status]}
                  </Badge>
                </TD>
                <TD className="text-right">
                  <span className="num font-medium text-ink">
                    {formatInt(run.matched)}
                  </span>
                </TD>
                <TD className="text-right">
                  <span
                    className={`num font-medium ${
                      run.unmatched > 0 ? "text-warning" : "text-ink-faint"
                    }`}
                  >
                    {formatInt(run.unmatched)}
                  </span>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Card>

      <Card
        title="Discrepancias"
        subtitle="Diferencias detectadas entre ledger y fuente externa"
        flush
      >
        <Table>
          <THead>
            <TH>ID</TH>
            <TH>Fuente</TH>
            <TH>Tipo</TH>
            <TH>Referencia externa</TH>
            <TH className="text-right">Monto</TH>
            <TH>Estado</TH>
            <TH className="text-right">Detectada</TH>
          </THead>
          <TBody>
            {discrepancies.map((discrepancy) => (
              <TR key={discrepancy.id}>
                <TD>
                  <span className="font-mono text-xs text-accent">
                    {discrepancy.id}
                  </span>
                </TD>
                <TD>
                  <span className="font-mono text-xs">{discrepancy.source}</span>
                </TD>
                <TD>
                  <Badge tone={TYPE_TONE[discrepancy.type]}>
                    {discrepancy.type}
                  </Badge>
                </TD>
                <TD>
                  <span className="font-mono text-xs">
                    {discrepancy.externalRef}
                  </span>
                </TD>
                <TD className="text-right">
                  <span className="num font-medium text-ink">
                    {formatMoneyWithAsset(
                      discrepancy.amountUnits,
                      discrepancy.asset,
                      discrepancy.exponent,
                    )}
                  </span>
                </TD>
                <TD>
                  <Badge tone={STATUS_TONE[discrepancy.status]} dot>
                    {STATUS_LABEL[discrepancy.status]}
                  </Badge>
                </TD>
                <TD className="text-right text-xs whitespace-nowrap">
                  {formatDateTime(discrepancy.detectedAt)}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Card>
    </div>
  );
}
