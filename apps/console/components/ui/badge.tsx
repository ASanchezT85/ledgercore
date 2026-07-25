import type { ReactNode } from "react";

export type BadgeTone =
  | "emerald"
  | "amber"
  | "red"
  | "sky"
  | "slate"
  | "violet";

const TONE_CLASSES: Record<BadgeTone, string> = {
  emerald: "bg-accent-soft text-accent border-accent/25",
  amber: "bg-warning-soft text-warning border-warning/25",
  red: "bg-danger-soft text-danger border-danger/25",
  sky: "bg-info-soft text-info border-info/25",
  slate: "bg-surface-raised text-ink-muted border-edge-strong",
  violet: "bg-[rgb(167_139_250/0.12)] text-[#a78bfa] border-[rgb(167_139_250/0.25)]",
};

export interface BadgeProps {
  tone?: BadgeTone;
  children: ReactNode;
  className?: string;
  /** Renders a small status dot before the label. */
  dot?: boolean;
}

export function Badge({
  tone = "slate",
  children,
  className = "",
  dot = false,
}: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] font-medium tracking-wide whitespace-nowrap ${TONE_CLASSES[tone]} ${className}`}
    >
      {dot && (
        <span
          className="size-1.5 rounded-full bg-current"
          aria-hidden="true"
        />
      )}
      {children}
    </span>
  );
}
