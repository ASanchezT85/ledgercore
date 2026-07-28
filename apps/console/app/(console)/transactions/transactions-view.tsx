"use client";

import { useCallback, useState } from "react";
import { ThinkingOrb } from "thinking-orbs";
import { DemoBadge } from "@/components/demo-badge";
import {
  ApiDownBanner,
  EmptyState,
  ErrorBanner,
  LoadingCard,
  useLiveData,
} from "@/components/live";
import {
  assetExponent,
  loadTransactionsPage,
  transactionTotals,
  type ApiTransaction,
  type LiveTransactionsPage,
} from "@/lib/live";
import { mockTransactions } from "@/lib/mock-data";
import type { Transaction } from "@/lib/types";
import { TransactionsExplorer } from "./transactions-explorer";

const FIRST_TXN_CURL = `curl -s -X POST $API/v1/transactions \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "ledger_id": "$LEDGER_ID",
    "idempotency_key": "dep-0001",
    "description": "Customer USD deposit",
    "postings": [
      { "account_id": "$CASH_ID",   "direction": "DEBIT",  "amount": { "asset": "USD", "amount": "10000" } },
      { "account_id": "$WALLET_ID", "direction": "CREDIT", "amount": { "asset": "USD", "amount": "9700" } },
      { "account_id": "$FEES_ID",   "direction": "CREDIT", "amount": { "asset": "USD", "amount": "300" } }
    ]
  }'`;

/** Maps the real API transaction into the explorer's view model. */
function toViewTransaction(
  txn: ApiTransaction,
  accountNames: Record<string, string>,
  ledgerNames: Record<string, string>,
): Transaction {
  const totals = transactionTotals(txn);
  const main = totals[0] ?? { asset: "USD", amount: "0" };
  const exponent = assetExponent(main.asset);
  return {
    id: txn.id,
    ledger: ledgerNames[txn.ledger_id] ?? txn.ledger_id,
    description: txn.description || txn.idempotency_key,
    // The API's "draft" maps to the explorer's "pending" bucket.
    status:
      txn.status === "draft"
        ? "pending"
        : txn.status === "reversed"
          ? "reversed"
          : "posted",
    amountUnits: BigInt(main.amount),
    asset: main.asset,
    exponent,
    createdAt: txn.created_at,
    idempotencyKey: txn.idempotency_key,
    postings: txn.postings.map((p) => ({
      account: accountNames[p.account_id] ?? p.account_id,
      direction: p.direction === "DEBIT" ? "debit" : "credit",
      amountUnits: BigInt(p.amount.amount),
      asset: p.amount.asset,
      exponent: assetExponent(p.amount.asset),
    })),
  };
}

function LiveTransactions({ initial }: { initial: LiveTransactionsPage }) {
  const [pages, setPages] = useState<LiveTransactionsPage[]>([initial]);
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState(false);

  const last = pages[pages.length - 1] ?? initial;
  const accountNames = Object.assign({}, ...pages.map((p) => p.accountNames));
  const ledgerNames = Object.assign({}, ...pages.map((p) => p.ledgerNames));
  const transactions = pages
    .flatMap((p) => p.transactions)
    .map((t) => toViewTransaction(t, accountNames, ledgerNames));

  const loadMore = useCallback(async () => {
    if (!last.nextCursor) return;
    setLoadingMore(true);
    setMoreError(false);
    try {
      const next = await loadTransactionsPage(last.nextCursor);
      setPages((prev) => [...prev, next]);
    } catch {
      setMoreError(true);
    } finally {
      setLoadingMore(false);
    }
  }, [last.nextCursor]);

  if (transactions.length === 0) {
    return (
      <EmptyState
        title="Todavía no hay transacciones"
        body="Cuando postees tu primer movimiento de doble partida aparecerá aquí, con sus postings y su idempotency key. Este es el curl canónico del depósito 100 / 97 / 3:"
        curl={FIRST_TXN_CURL}
        curlLabel="POST /v1/transactions"
      />
    );
  }

  return (
    <>
      <TransactionsExplorer transactions={transactions} />
      {last.nextCursor && (
        <div className="flex justify-center">
          <button
            type="button"
            onClick={() => void loadMore()}
            disabled={loadingMore}
            className="inline-flex h-9 items-center gap-2 rounded-(--radius-control) border border-edge-strong px-4 text-xs font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent disabled:opacity-50"
          >
            {loadingMore && (
              <ThinkingOrb state="solving" size={20} aria-hidden="true" />
            )}
            {loadingMore ? "Cargando…" : "Cargar más"}
          </button>
        </div>
      )}
      {moreError && (
        <p className="text-center text-xs text-danger">
          No se pudo cargar la siguiente página. Intenta de nuevo.
        </p>
      )}
    </>
  );
}

export function TransactionsView() {
  const state = useLiveData(() => loadTransactionsPage());

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink">
            Transacciones
          </h1>
          <p className="mt-0.5 text-sm text-ink-faint">
            Movimientos de doble partida: cada transacción balancea débitos y
            créditos por asset
          </p>
        </div>
        <DemoBadge demo={state.status === "demo"} />
      </div>

      {state.status === "loading" && <LoadingCard orb="solving" />}
      {state.status === "demo" && (
        <TransactionsExplorer transactions={mockTransactions} />
      )}
      {state.status === "down" && <ApiDownBanner onRetry={state.reload} />}
      {state.status === "error" && (
        <ErrorBanner message={state.message} onRetry={state.reload} />
      )}
      {state.status === "live" && (
        <LiveTransactions initial={state.data} />
      )}
    </div>
  );
}
