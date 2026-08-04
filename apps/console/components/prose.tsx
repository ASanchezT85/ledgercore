/** Shared long-form typography for blog posts. */

export function P({ children }: { children: React.ReactNode }) {
  return (
    <p className="mt-6 text-[1.0625rem] leading-[1.85] text-ink-muted">
      {children}
    </p>
  );
}

export function Lead({ children }: { children: React.ReactNode }) {
  return (
    <p className="mt-8 text-lg leading-[1.8] text-ink">{children}</p>
  );
}

export function H2({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mt-12 text-2xl font-semibold tracking-tight text-ink">
      {children}
    </h2>
  );
}

export function Strong({ children }: { children: React.ReactNode }) {
  return <strong className="font-semibold text-ink">{children}</strong>;
}

export function Code({ children }: { children: React.ReactNode }) {
  return (
    <code className="rounded bg-surface-raised px-1.5 py-0.5 font-mono text-[0.85em] text-ink">
      {children}
    </code>
  );
}

export function UL({ children }: { children: React.ReactNode }) {
  return (
    <ul className="mt-6 space-y-3 pl-5 text-[1.0625rem] leading-[1.85] text-ink-muted [&>li]:list-disc [&>li]:marker:text-accent">
      {children}
    </ul>
  );
}

export function CodeBlock({
  code,
  label = "SQL",
  hint = "postgres",
}: {
  code: string;
  label?: string;
  hint?: string;
}) {
  return (
    <div className="mt-6 overflow-hidden rounded-(--radius-card) border border-edge bg-surface">
      <div className="flex items-center justify-between border-b border-edge px-4 py-2.5">
        <span className="font-mono text-[11px] text-ink-faint">{label}</span>
        <span className="text-[10px] tracking-widest text-ink-faint uppercase">
          {hint}
        </span>
      </div>
      <pre className="overflow-x-auto p-4 font-mono text-[13px] leading-relaxed text-ink-muted">
        <code>{code}</code>
      </pre>
    </div>
  );
}

/** Closing note: at most one product mention per piece, per the brand guide. */
export function Outro({ children }: { children: React.ReactNode }) {
  return (
    <p className="mt-12 rounded-(--radius-card) border border-edge bg-surface/70 p-5 text-sm leading-relaxed text-ink-muted">
      {children}
    </p>
  );
}
