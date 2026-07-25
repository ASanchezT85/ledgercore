import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

export interface StatTileProps {
  label: string;
  value: ReactNode;
  hint?: string;
  icon: LucideIcon;
  /** Accent treatment for the icon chip. */
  tone?: "emerald" | "amber" | "red" | "sky";
}

const TONE_CLASSES: Record<NonNullable<StatTileProps["tone"]>, string> = {
  emerald: "bg-accent-soft text-accent",
  amber: "bg-warning-soft text-warning",
  red: "bg-danger-soft text-danger",
  sky: "bg-info-soft text-info",
};

export function StatTile({
  label,
  value,
  hint,
  icon: Icon,
  tone = "emerald",
}: StatTileProps) {
  return (
    <div className="rounded-(--radius-card) border border-edge bg-surface/80 p-5 shadow-[0_1px_0_rgb(255_255_255/0.03)_inset]">
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-semibold tracking-wider text-ink-faint uppercase">
          {label}
        </p>
        <span
          className={`inline-flex size-8 items-center justify-center rounded-lg ${TONE_CLASSES[tone]}`}
        >
          <Icon size={16} strokeWidth={2} aria-hidden="true" />
        </span>
      </div>
      <p className="num mt-3 text-2xl font-semibold tracking-tight text-ink">
        {value}
      </p>
      {hint && <p className="mt-1 text-xs text-ink-faint">{hint}</p>}
    </div>
  );
}
