import type { Metadata } from "next";
import { QuickstartView } from "./quickstart-view";

export const metadata: Metadata = {
  title: "Quickstart — de 0 a tu primera transacción",
  description:
    "De 0 a tu primera transacción de doble partida en 10 minutos: sandbox, token, ledger, cuentas, depósito 100/97/3 y replay idempotente.",
};

export default function QuickstartPage() {
  return <QuickstartView />;
}
