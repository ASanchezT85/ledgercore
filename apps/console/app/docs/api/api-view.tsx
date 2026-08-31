"use client";

// Interactive API reference for the four OpenAPI contracts.
//
// Rendering: Scalar's standalone browser bundle, self-hosted at
// /vendor/scalar.standalone.js (copied from node_modules by
// scripts/prepare-docs-assets.mjs at prebuild). The specs are served
// same-origin from /openapi/*.v1.yaml — no external CDN involved, so the
// page works in the standalone production build without internet access.

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { BrandLockup } from "@/components/logo";

declare global {
  interface Window {
    Scalar?: {
      createApiReference: (el: string | HTMLElement, config: unknown) => unknown;
    };
  }
}

const SOURCES = [
  { slug: "ledger", title: "Ledger Core", url: "/openapi/ledger.v1.yaml", default: true },
  { slug: "identity", title: "Identity", url: "/openapi/identity.v1.yaml" },
  { slug: "reconciliation", title: "Reconciliation", url: "/openapi/reconciliation.v1.yaml" },
  { slug: "webhooks", title: "Webhooks", url: "/openapi/webhooks.v1.yaml" },
];

export function ApiReferenceView() {
  const containerRef = useRef<HTMLDivElement>(null);
  const mounted = useRef(false);
  const [error, setError] = useState(false);

  useEffect(() => {
    if (mounted.current) return;
    mounted.current = true;

    const init = () => {
      if (!window.Scalar || !containerRef.current) {
        setError(true);
        return;
      }
      window.Scalar.createApiReference(containerRef.current, {
        sources: SOURCES,
        // Self-contained: no font fetch from Scalar's CDN, no request proxy.
        withDefaultFonts: false,
        proxyUrl: "",
        forceDarkModeState: "dark",
        hideDarkModeToggle: true,
        hideTestRequestButton: false,
        theme: "deepSpace",
        metaData: { title: "LedgerCore API Reference" },
      });
    };

    if (window.Scalar) {
      init();
      return;
    }
    const script = document.createElement("script");
    script.src = "/vendor/scalar.standalone.js";
    script.async = true;
    script.onload = init;
    script.onerror = () => setError(true);
    document.body.appendChild(script);
  }, []);

  return (
    <div className="min-h-screen">
      <header className="flex w-full items-center justify-between border-b border-edge px-6 py-4">
        <div className="flex items-center gap-4">
          <Link href="/" aria-label="LedgerCore">
            <BrandLockup size={26} />
          </Link>
          <Link
            href="/docs"
            className="inline-flex items-center gap-1.5 text-sm text-ink-muted transition-colors hover:text-accent"
          >
            <ArrowLeft size={14} aria-hidden="true" />
            Docs
          </Link>
        </div>
        <span className="font-mono text-xs text-ink-faint">
          localhost:8080
        </span>
      </header>
      {error && (
        <div className="mx-auto max-w-xl px-6 py-16 text-center">
          <p className="text-sm text-ink-muted">
            No se pudo cargar el visor interactivo. / The interactive viewer failed to load.
          </p>
          <ul className="mt-4 space-y-1 font-mono text-xs">
            {SOURCES.map((s) => (
              <li key={s.slug}>
                <a className="text-accent hover:underline" href={s.url}>
                  {s.title} — {s.url}
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div ref={containerRef} />
    </div>
  );
}
