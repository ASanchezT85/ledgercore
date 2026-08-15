import type { Metadata } from "next";
import { Inter, Instrument_Serif } from "next/font/google";
import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
  fallback: ["system-ui", "Segoe UI", "Helvetica Neue", "Arial", "sans-serif"],
});

/**
 * Serif de titular. Un producto de contabilidad por partida doble hereda el
 * vocabulario visual del libro contable; el serif lo dice sin decorar nada.
 * Sólo pesa 400 — los titulares no llevan `font-semibold`, el contraste del
 * tipo hace ese trabajo.
 */
const instrumentSerif = Instrument_Serif({
  subsets: ["latin"],
  weight: "400",
  variable: "--font-instrument-serif",
  display: "swap",
  fallback: ["Georgia", "Times New Roman", "serif"],
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
    <html
      lang="es"
      className={`${inter.variable} ${instrumentSerif.variable}`}
    >
      <body className="bg-ambient min-h-screen font-sans antialiased">
        {children}
      </body>
    </html>
  );
}
