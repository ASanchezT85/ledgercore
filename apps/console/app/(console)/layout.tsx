import type { ReactNode } from "react";
import { Sidebar } from "@/components/ui/sidebar";

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen">
      <Sidebar />
      <div className="flex min-h-screen flex-col pl-64">
        <main className="mx-auto w-full max-w-7xl flex-1 px-8 py-8">
          {children}
        </main>
        <footer className="border-t border-edge px-8 py-4">
          <p className="text-[11px] tracking-wide text-ink-faint">
            LedgerCore — infraestructura financiera
          </p>
        </footer>
      </div>
    </div>
  );
}
