import type { Metadata } from "next";
import { TransactionsView } from "./transactions-view";

export const metadata: Metadata = {
  title: "Transacciones",
};

export default function TransactionsPage() {
  return <TransactionsView />;
}
