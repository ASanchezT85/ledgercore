/**
 * LedgerCore logomark: a stylized isometric cube/ledger book. Pure inline
 * SVG so it ships with zero assets and inherits the accent gradient.
 */
export function Logomark({ size = 28 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <defs>
        <linearGradient id="lc-grad" x1="4" y1="4" x2="28" y2="28">
          <stop offset="0%" stopColor="#34d399" />
          <stop offset="100%" stopColor="#0d9488" />
        </linearGradient>
      </defs>
      {/* top face */}
      <path
        d="M16 3 28 9.5 16 16 4 9.5 16 3Z"
        fill="url(#lc-grad)"
        opacity="0.95"
      />
      {/* left face (ledger pages) */}
      <path d="M4 9.5 16 16v13L4 22.5v-13Z" fill="url(#lc-grad)" opacity="0.55" />
      {/* right face */}
      <path d="M28 9.5 16 16v13l12-6.5v-13Z" fill="url(#lc-grad)" opacity="0.75" />
      {/* page lines on the right face */}
      <path
        d="m18.5 18.4 7-3.8M18.5 21.6l7-3.8M18.5 24.8l7-3.8"
        stroke="#022c22"
        strokeWidth="1.1"
        strokeLinecap="round"
        opacity="0.5"
      />
    </svg>
  );
}

export function BrandLockup({ size = 28 }: { size?: number }) {
  return (
    <span className="inline-flex items-center gap-2.5">
      <Logomark size={size} />
      <span className="text-lg font-semibold tracking-tight text-ink">
        Ledger<span className="text-accent">Core</span>
      </span>
    </span>
  );
}
