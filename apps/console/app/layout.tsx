import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
  fallback: ["system-ui", "Segoe UI", "Helvetica Neue", "Arial", "sans-serif"],
});

export const metadata: Metadata = {
  title: {
    default: "LedgerCore Console",
    template: "%s · LedgerCore Console",
  },
  description:
    "Consola de operaciones de LedgerCore: ledgers, transacciones, conciliación y herramientas para desarrolladores.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="es" className={inter.variable}>
      <body className="bg-ambient min-h-screen font-sans antialiased">
        {children}
      </body>
    </html>
  );
}
