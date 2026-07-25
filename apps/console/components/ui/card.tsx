import type { ReactNode } from "react";

export interface CardProps {
  title?: string;
  subtitle?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  /** Removes inner padding on the body (useful for full-bleed tables). */
  flush?: boolean;
}

export function Card({
  title,
  subtitle,
  actions,
  children,
  className = "",
  flush = false,
}: CardProps) {
  return (
    <section
      className={`rounded-(--radius-card) border border-edge bg-surface/80 shadow-[0_1px_0_rgb(255_255_255/0.03)_inset,0_8px_24px_rgb(2_6_12/0.45)] backdrop-blur-sm ${className}`}
    >
      {(title || actions) && (
        <header className="flex items-start justify-between gap-4 border-b border-edge px-5 py-4">
          <div>
            {title && (
              <h2 className="text-sm font-semibold tracking-tight text-ink">
                {title}
              </h2>
            )}
            {subtitle && (
              <p className="mt-0.5 text-xs text-ink-faint">{subtitle}</p>
            )}
          </div>
          {actions && <div className="shrink-0">{actions}</div>}
        </header>
      )}
      <div className={flush ? "" : "px-5 py-4"}>{children}</div>
    </section>
  );
}
