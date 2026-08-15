"use client";

import Link from "next/link";
import { BrandLockup } from "@/components/logo";
import { LangToggle, useLang } from "@/components/language";

/**
 * Cabecera compartida de las páginas públicas (landing, blog, login, signup).
 * Garantiza que desde cualquiera de ellas se pueda volver al inicio y cambiar
 * de idioma — no asumimos que quien llega lee español.
 */
export function PublicHeader({
  hideLogin = false,
  showHome = false,
}: {
  hideLogin?: boolean;
  showHome?: boolean;
}) {
  const { t } = useLang();
  // `hidden sm:inline-flex`: en 375px los seis items partian la cabecera en tres
  // filas (174px, el 21% del viewport) y la primera pantalla era casi toda nav.
  const linkClass =
    "hidden h-11 items-center rounded-(--radius-control) px-4 text-sm text-ink-muted transition-colors hover:text-ink sm:inline-flex";

  return (
    <header className="mx-auto flex w-full max-w-6xl items-center justify-between gap-4 px-6 py-6">
      <Link href="/" aria-label={t.nav.homeAriaLabel}>
        <BrandLockup size={30} />
      </Link>
      <nav
        className="flex flex-wrap items-center justify-end gap-1"
        aria-label={t.nav.ariaLabel}
      >
        <LangToggle />
        {showHome && (
          <Link href="/" className={linkClass}>
            {t.nav.home}
          </Link>
        )}
        <Link href="/blog" className={linkClass}>
          {t.nav.blog}
        </Link>
        <Link href="/docs" className={linkClass}>
          {t.nav.docs}
        </Link>
        <Link href="/docs/quickstart" className={linkClass}>
          {t.nav.quickstart}
        </Link>
        {!hideLogin && (
          <Link href="/login" className={linkClass}>
            {t.nav.login}
          </Link>
        )}
        <Link
          href="/signup"
          className="inline-flex h-11 items-center rounded-(--radius-control) border border-edge-strong px-4 text-sm font-medium text-ink transition-colors hover:border-accent/60 hover:text-accent"
        >
          {t.nav.trySandbox}
        </Link>
      </nav>
    </header>
  );
}

export function PublicFooter() {
  const { t } = useLang();
  return (
    <footer className="border-t border-edge">
      <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-8">
        <Link href="/" aria-label={t.nav.homeAriaLabel}>
          <BrandLockup size={22} />
        </Link>
        <p className="text-[11px] tracking-wide text-ink-faint">
          {t.footer.tagline}
        </p>
      </div>
    </footer>
  );
}
