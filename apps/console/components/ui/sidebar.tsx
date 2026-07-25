"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ArrowLeftRight,
  BookOpenText,
  Code2,
  LayoutDashboard,
  LogOut,
  Scale,
} from "lucide-react";
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";

const NAV_ITEMS = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard },
  { href: "/ledgers", label: "Ledgers", icon: BookOpenText },
  { href: "/transactions", label: "Transacciones", icon: ArrowLeftRight },
  { href: "/reconciliation", label: "Conciliación", icon: Scale },
  { href: "/developers", label: "Developers", icon: Code2 },
] as const;

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="fixed inset-y-0 left-0 z-30 flex w-64 flex-col border-r border-edge bg-surface/60 backdrop-blur-md">
      <div className="flex items-center justify-between px-5 pt-5 pb-4">
        <Link href="/" aria-label="LedgerCore — dashboard">
          <BrandLockup />
        </Link>
      </div>

      <div className="px-5 pb-4">
        <div className="flex items-center justify-between rounded-lg border border-edge bg-surface-raised px-3 py-2">
          <div className="min-w-0">
            <p className="truncate text-xs font-medium text-ink">
              Acme Fintech, Inc.
            </p>
            <p className="text-[10px] text-ink-faint">tenant · producción</p>
          </div>
          <Badge tone="emerald" dot>
            live
          </Badge>
        </div>
      </div>

      <nav className="flex-1 space-y-0.5 px-3" aria-label="Principal">
        {NAV_ITEMS.map(({ href, label, icon: Icon }) => {
          const active =
            href === "/" ? pathname === "/" : pathname.startsWith(href);
          return (
            <Link
              key={href}
              href={href}
              aria-current={active ? "page" : undefined}
              className={`group flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors ${
                active
                  ? "bg-accent-soft font-medium text-accent"
                  : "text-ink-muted hover:bg-surface-hover hover:text-ink"
              }`}
            >
              <Icon
                size={17}
                strokeWidth={2}
                className={active ? "text-accent" : "text-ink-faint group-hover:text-ink-muted"}
                aria-hidden="true"
              />
              {label}
              {active && (
                <span
                  className="ml-auto h-4 w-0.5 rounded-full bg-accent"
                  aria-hidden="true"
                />
              )}
            </Link>
          );
        })}
      </nav>

      <div className="border-t border-edge px-3 py-4">
        <div className="flex items-center gap-3 rounded-lg px-3 py-2">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-emerald-500/30 to-teal-500/30 text-xs font-semibold text-accent">
            MF
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs font-medium text-ink">
              María Fernández
            </p>
            <p className="truncate text-[10px] text-ink-faint">
              finance-ops@acme.com
            </p>
          </div>
          <Link
            href="/login"
            className="text-ink-faint transition-colors hover:text-danger"
            aria-label="Cerrar sesión"
          >
            <LogOut size={15} aria-hidden="true" />
          </Link>
        </div>
      </div>
    </aside>
  );
}
