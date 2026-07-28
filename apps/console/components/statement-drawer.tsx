"use client";

/**
 * Statement drawer: fetches GET /v1/statements for one account and renders
 * the period, opening/closing balances and entries with running balance.
 * Client-side CSV export, no extra dependencies.
 */

import { Download, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { formatMoneyWithAsset } from "@/lib/format";
import {
  type ApiStatement,
  type ApiStatementEntry,
  assetExponent,
  getStatement,
} from "@/lib/live";

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; statement: ApiStatement; entries: ApiStatementEntry[] };

function toCsv(statement: ApiStatement, entries: ApiStatementEntry[]): string {
  const esc = (v: string) => `"${v.replace(/"/g, '""')}"`;
  const lines = [
    [
      "posting_id",
      "transaction_id",
      "reference",
      "direction",
      "asset",
      "amount",
      "effective_at",
      "running_balance",
    ].join(","),
    ...entries.map((e) =>
      [
        e.id,
        e.transaction_id,
        esc(e.reference ?? ""),
        e.direction,
        e.amount.asset,
        e.amount.amount,
        e.effective_at,
        e.running_balance,
      ].join(","),
    ),
  ];
  return lines.join("\n");
}

function downloadCsv(statement: ApiStatement, entries: ApiStatementEntry[]) {
  const blob = new Blob([toCsv(statement, entries)], {
    type: "text/csv;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `statement-${statement.account.name.replace(/[^a-z0-9_-]+/gi, "_")}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

function BalanceRow({
  label,
  balances,
}: {
  label: string;
  balances: ApiStatement["opening_balance"];
}) {
  return (
    <div className="flex items-center justify-between border-b border-edge py-2">
      <span className="text-xs font-semibold tracking-wider text-ink-faint uppercase">
        {label}
      </span>
      <span className="flex flex-col items-end gap-0.5">
        {balances.length === 0 && (
          <span className="text-xs text-ink-faint">—</span>
        )}
        {balances.map((b) => (
          <span key={b.asset} className="num text-[13px] font-medium text-ink">
            {formatMoneyWithAsset(BigInt(b.amount), b.asset, b.exponent)}
          </span>
        ))}
      </span>
    </div>
  );
}

export function StatementDrawer({
  accountId,
  accountName,
  onClose,
}: {
  accountId: string;
  accountName: string;
  onClose: () => void;
}) {
  const [state, setState] = useState<State>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        // Follow the cursor so the export covers the whole period.
        const first = await getStatement(accountId);
        const entries = [...first.entries];
        let cursor = first.next_cursor;
        while (cursor && entries.length < 1000) {
          const page = await getStatement(accountId, cursor);
          entries.push(...page.entries);
          cursor = page.next_cursor;
        }
        if (!cancelled) setState({ status: "ready", statement: first, entries });
      } catch (err) {
        if (!cancelled)
          setState({
            status: "error",
            message: err instanceof Error ? err.message : "unknown_error",
          });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [accountId]);

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <button
        type="button"
        aria-label="Cerrar"
        className="absolute inset-0 cursor-default bg-black/60"
        onClick={onClose}
      />
      <aside className="relative flex h-full w-full max-w-xl flex-col overflow-y-auto border-l border-edge bg-surface p-5 shadow-2xl">
        <header className="mb-4 flex items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold tracking-tight text-ink">
              Estado de cuenta
            </h2>
            <p className="mt-0.5 font-mono text-xs text-ink-faint">
              {accountName}
            </p>
          </div>
          <div className="flex items-center gap-2">
            {state.status === "ready" && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => downloadCsv(state.statement, state.entries)}
              >
                <Download size={13} aria-hidden="true" /> CSV
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={onClose} aria-label="Cerrar drawer">
              <X size={14} aria-hidden="true" />
            </Button>
          </div>
        </header>

        {state.status === "loading" && (
          <p className="py-8 text-center text-sm text-ink-faint">Cargando…</p>
        )}
        {state.status === "error" && (
          <p className="py-8 text-center text-sm text-rose-400">
            No se pudo cargar el statement: {state.message}
          </p>
        )}
        {state.status === "ready" && (
          <>
            <p className="mb-3 text-xs text-ink-faint">
              Período{" "}
              {new Date(state.statement.period.from).toLocaleDateString()} —{" "}
              {new Date(state.statement.period.to).toLocaleDateString()} (últimos
              30 días)
            </p>
            <BalanceRow
              label="Balance inicial"
              balances={state.statement.opening_balance}
            />
            <ul>
              {state.entries.length === 0 && (
                <li className="py-6 text-center text-xs text-ink-faint">
                  Sin movimientos en el período
                </li>
              )}
              {state.entries.map((e) => (
                <li
                  key={e.id}
                  className="flex items-center justify-between gap-3 border-b border-edge py-2"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-xs text-ink">
                      {e.reference || e.transaction_id}
                    </p>
                    <p className="text-[11px] text-ink-faint">
                      {new Date(e.effective_at).toLocaleString()}
                    </p>
                  </div>
                  <div className="flex shrink-0 flex-col items-end">
                    <span
                      className={`num text-[13px] font-medium ${
                        e.direction === "CREDIT" ? "text-accent" : "text-ink"
                      }`}
                    >
                      {e.direction === "CREDIT" ? "+" : "−"}
                      {formatMoneyWithAsset(
                        BigInt(e.amount.amount),
                        e.amount.asset,
                        assetExponent(e.amount.asset),
                      )}
                    </span>
                    <span className="num text-[11px] text-ink-faint">
                      saldo{" "}
                      {formatMoneyWithAsset(
                        BigInt(e.running_balance),
                        e.amount.asset,
                        assetExponent(e.amount.asset),
                      )}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
            <BalanceRow
              label="Balance final"
              balances={state.statement.closing_balance}
            />
          </>
        )}
      </aside>
    </div>
  );
}
