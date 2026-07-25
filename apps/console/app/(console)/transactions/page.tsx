import type { Metadata } from "next";
import { DemoBadge } from "@/components/demo-badge";
import { getTransactions } from "@/lib/api";
import { TransactionsExplorer } from "./transactions-explorer";

export const metadata: Metadata = {
  title: "Transacciones",
};

export const dynamic = "force-dynamic";

export default async function TransactionsPage() {
  const { data: transactions, demo } = await getTransactions();

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
        <DemoBadge demo={demo} />
      </div>

      <TransactionsExplorer transactions={transactions} />
    </div>
  );
}
