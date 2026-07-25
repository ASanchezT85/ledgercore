import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight, KeyRound, Lock, Mail } from "lucide-react";
import { BrandLockup } from "@/components/logo";
import { Badge } from "@/components/ui/badge";

export const metadata: Metadata = {
  title: "Iniciar sesión",
};

export default function LoginPage() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex flex-col items-center gap-3 text-center">
          <BrandLockup size={40} />
          <p className="text-sm text-ink-faint">
            Consola de operaciones · ledger de doble partida como servicio
          </p>
        </div>

        <div className="rounded-(--radius-card) border border-edge bg-surface/80 p-6 shadow-[0_1px_0_rgb(255_255_255/0.04)_inset,0_16px_48px_rgb(2_6_12/0.6)] backdrop-blur-md">
          <div className="mb-5 flex items-center justify-between">
            <h1 className="text-sm font-semibold text-ink">Iniciar sesión</h1>
            <Badge tone="sky" className="gap-1">
              <KeyRound size={11} aria-hidden="true" />
              SSO próximamente
            </Badge>
          </div>

          <fieldset disabled className="space-y-3 opacity-55">
            <label className="block">
              <span className="mb-1.5 block text-xs font-medium text-ink-muted">
                Correo corporativo
              </span>
              <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                <Mail size={14} className="text-ink-faint" aria-hidden="true" />
                <input
                  type="email"
                  placeholder="tu@empresa.com"
                  className="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
                  autoComplete="off"
                />
              </span>
            </label>

            <label className="block">
              <span className="mb-1.5 block text-xs font-medium text-ink-muted">
                Contraseña
              </span>
              <span className="flex items-center gap-2 rounded-(--radius-control) border border-edge-strong bg-surface-raised px-3 py-2.5">
                <Lock size={14} className="text-ink-faint" aria-hidden="true" />
                <input
                  type="password"
                  placeholder="••••••••••••"
                  className="w-full bg-transparent text-sm text-ink outline-none placeholder:text-ink-faint"
                  autoComplete="off"
                />
              </span>
            </label>

            <button
              type="button"
              className="h-10 w-full rounded-(--radius-control) border border-edge-strong text-sm font-medium text-ink-faint"
            >
              Continuar
            </button>
          </fieldset>

          <p className="mt-3 text-center text-[11px] text-ink-faint">
            La autenticación con identidad corporativa (SSO / EdDSA JWT) se
            habilitará con el servicio de identity.
          </p>

          <div className="my-5 flex items-center gap-3">
            <span className="h-px flex-1 bg-edge" aria-hidden="true" />
            <span className="text-[10px] tracking-widest text-ink-faint uppercase">
              o
            </span>
            <span className="h-px flex-1 bg-edge" aria-hidden="true" />
          </div>

          <Link
            href="/"
            className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-(--radius-control) bg-accent-deep text-sm font-semibold text-[#03150e] shadow-[0_0_24px_rgb(16_185_129/0.3)] transition-colors hover:bg-accent"
          >
            Entrar en modo demo
            <ArrowRight size={15} aria-hidden="true" />
          </Link>
        </div>

        <p className="mt-6 text-center text-[11px] text-ink-faint">
          LedgerCore — infraestructura financiera
        </p>
      </div>
    </div>
  );
}
