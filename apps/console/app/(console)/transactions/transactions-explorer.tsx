"use client";

import { useMemo, useState } from "react";
import { ArrowDownLeft, ArrowUpRight, Filter, X } from "lucide-react";
import { Badge, type BadgeTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import {
  formatDateTime,
  formatMoneyWithAsset,
} from "@/lib/format";
import type { Transaction, TransactionStatus } from "@/lib/types";

const STATUS_TONE: Record<TransactionStatus, BadgeTone> = {
  posted: "emerald",
  pending: "amber",
  reversed: "red",
};

const STATUS_OPTIONS: Array<{ value: TransactionStatus | "all"; label: string }> =
  [
    { value: "all", label: "Todos los estados" },
    { value: "posted", label: "Posted" },
    { value: "pending", label: "Pending" },
    { value: "reversed", label: "Reversed" },
  ];

const SELECT_CLASSES =
  "h-9 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 text-xs text-ink transition-colors focus-visible:border-accent/60";

export function TransactionsExplorer({
  transactions,
}: {
  transactions: Transaction[];
}) {
  const [statusFilter, setStatusFilter] = useState<TransactionStatus | "all">(
    "all",
  );
  const [assetFilter, setAssetFilter] = useState<string>("all");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const assets = useMemo(
    () => [...new Set(transactions.map((txn) => txn.asset))].sort(),
    [transactions],
  );

  const filtered = useMemo(
    () =>
      transactions.filter(
        (txn) =>
          (statusFilter === "all" || txn.status === statusFilter) &&
          (assetFilter === "all" || txn.asset === assetFilter),
      ),
    [transactions, statusFilter, assetFilter],
  );

  const selected = useMemo(
    () => transactions.find((txn) => txn.id === selectedId) ?? null,
    [transactions, selectedId],
  );

  return (
    <>
      <Card flush>
        <div className="flex flex-wrap items-center gap-3 border-b border-edge px-5 py-3">
          <span className="inline-flex items-center gap-1.5 text-xs font-medium text-ink-faint">
            <Filter size={13} aria-hidden="true" />
            Filtros
          </span>
          <select
            className={SELECT_CLASSES}
            value={statusFilter}
            onChange={(event) =>
              setStatusFilter(event.target.value as TransactionStatus | "all")
            }
            aria-label="Filtrar por estado"
          >
            {STATUS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <select
            className={SELECT_CLASSES}
            value={assetFilter}
            onChange={(event) => setAssetFilter(event.target.value)}
            aria-label="Filtrar por asset"
          >
            <option value="all">Todos los assets</option>
            {assets.map((asset) => (
              <option key={asset} value={asset}>
                {asset}
              </option>
            ))}
          </select>
          <span className="ml-auto text-xs text-ink-faint">
            {filtered.length} de {transactions.length} transacciones
          </span>
        </div>

        <Table>
          <THead>
            <TH>ID</TH>
            <TH>Descripción</TH>
            <TH>Ledger</TH>
            <TH>Estado</TH>
            <TH className="text-right">Monto</TH>
            <TH className="text-right">Fecha</TH>
          </THead>
          <TBody>
            {filtered.map((txn) => (
              <TR
                key={txn.id}
                onClick={() => setSelectedId(txn.id)}
                selected={txn.id === selectedId}
              >
                <TD>
                  <span className="font-mono text-xs text-accent">{txn.id}</span>
                </TD>
                <TD>
                  <span className="font-medium text-ink">{txn.description}</span>
                </TD>
                <TD>
                  <span className="font-mono text-xs">{txn.ledger}</span>
                </TD>
                <TD>
                  <Badge tone={STATUS_TONE[txn.status]} dot>
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

        {filtered.length === 0 && (
          <p className="px-5 py-10 text-center text-sm text-ink-faint">
            Ninguna transacción coincide con los filtros seleccionados.
          </p>
        )}
      </Card>

      {/* Detail drawer */}
      {selected && (
        <div className="fixed inset-0 z-40" role="dialog" aria-modal="true">
          <button
            type="button"
            className="absolute inset-0 bg-black/50 backdrop-blur-[2px]"
            aria-label="Cerrar detalle"
            onClick={() => setSelectedId(null)}
          />
          <aside className="absolute inset-y-0 right-0 flex w-full max-w-lg flex-col overflow-y-auto border-l border-edge bg-surface shadow-2xl">
            <header className="sticky top-0 flex items-start justify-between gap-4 border-b border-edge bg-surface/95 px-6 py-5 backdrop-blur">
              <div>
                <p className="font-mono text-xs text-accent">{selected.id}</p>
                <h2 className="mt-1 text-base font-semibold text-ink">
                  {selected.description}
                </h2>
                <p className="mt-1 text-xs text-ink-faint">
                  {formatDateTime(selected.createdAt)} · ledger{" "}
                  <span className="font-mono">{selected.ledger}</span>
                </p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSelectedId(null)}
                aria-label="Cerrar"
              >
                <X size={16} aria-hidden="true" />
              </Button>
            </header>

            <div className="space-y-6 px-6 py-5">
              <div className="grid grid-cols-2 gap-4">
                <div className="rounded-lg border border-edge bg-surface-raised px-4 py-3">
                  <p className="text-[10px] font-semibold tracking-wider text-ink-faint uppercase">
                    Estado
                  </p>
                  <div className="mt-1.5">
                    <Badge tone={STATUS_TONE[selected.status]} dot>
                      {selected.status}
                    </Badge>
                  </div>
                </div>
                <div className="rounded-lg border border-edge bg-surface-raised px-4 py-3">
                  <p className="text-[10px] font-semibold tracking-wider text-ink-faint uppercase">
                    Monto principal
                  </p>
                  <p className="num mt-1.5 text-sm font-semibold text-ink">
                    {formatMoneyWithAsset(
                      selected.amountUnits,
                      selected.asset,
                      selected.exponent,
                    )}
                  </p>
                </div>
              </div>

              <div>
                <h3 className="mb-2 text-xs font-semibold tracking-wider text-ink-faint uppercase">
                  Postings (débito / crédito)
                </h3>
                <ul className="divide-y divide-edge rounded-lg border border-edge">
                  {selected.postings.map((posting, index) => (
                    <li
                      key={`${posting.account}-${index}`}
                      className="flex items-center justify-between gap-3 px-4 py-3"
                    >
                      <div className="flex min-w-0 items-center gap-2.5">
                        {posting.direction === "debit" ? (
                          <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-info-soft text-info">
                            <ArrowDownLeft size={13} aria-hidden="true" />
                          </span>
                        ) : (
                          <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent">
                            <ArrowUpRight size={13} aria-hidden="true" />
                          </span>
                        )}
                        <div className="min-w-0">
                          <p className="truncate font-mono text-xs text-ink">
                            {posting.account}
                          </p>
                          <p className="text-[10px] tracking-wider text-ink-faint uppercase">
                            {posting.direction}
                          </p>
                        </div>
                      </div>
                      <span
                        className={`num shrink-0 text-sm font-medium ${
                          posting.direction === "debit"
                            ? "text-info"
                            : "text-accent"
                        }`}
                      >
                        {posting.direction === "debit" ? "+" : "−"}
                        {formatMoneyWithAsset(
                          posting.amountUnits,
                          posting.asset,
                          posting.exponent,
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
                <p className="mt-2 text-[11px] text-ink-faint">
                  Débitos y créditos balancean por asset — invariante del motor
                  de doble partida.
                </p>
              </div>

              <div className="rounded-lg border border-edge bg-surface-raised px-4 py-3">
                <p className="text-[10px] font-semibold tracking-wider text-ink-faint uppercase">
                  Idempotency key
                </p>
                <p className="mt-1 font-mono text-xs text-ink-muted">
                  {selected.idempotencyKey}
                </p>
              </div>
            </div>
          </aside>
        </div>
      )}
    </>
  );
}
