import type { ReactNode, ThHTMLAttributes, TdHTMLAttributes } from "react";

/** Themed table primitives. Wrap in <Card flush> for full-bleed layout. */

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">{children}</table>
    </div>
  );
}

export function THead({ children }: { children: ReactNode }) {
  return (
    <thead>
      <tr className="border-b border-edge text-left">{children}</tr>
    </thead>
  );
}

export function TH({
  className = "",
  children,
  ...props
}: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      className={`px-5 py-3 text-[11px] font-semibold tracking-wider text-ink-faint uppercase ${className}`}
      {...props}
    >
      {children}
    </th>
  );
}

export function TBody({ children }: { children: ReactNode }) {
  return <tbody className="divide-y divide-edge">{children}</tbody>;
}

export function TR({
  children,
  onClick,
  selected = false,
}: {
  children: ReactNode;
  onClick?: () => void;
  selected?: boolean;
}) {
  const interactive = onClick
    ? "cursor-pointer transition-colors hover:bg-surface-hover"
    : "";
  const highlight = selected ? "bg-accent-soft/60" : "";
  return (
    <tr className={`${interactive} ${highlight}`} onClick={onClick}>
      {children}
    </tr>
  );
}

export function TD({
  className = "",
  children,
  ...props
}: TdHTMLAttributes<HTMLTableCellElement>) {
  return (
    <td className={`px-5 py-3 align-middle text-ink-muted ${className}`} {...props}>
      {children}
    </td>
  );
}
