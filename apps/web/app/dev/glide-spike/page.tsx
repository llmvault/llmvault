"use client";

import dynamic from "next/dynamic";

/**
 * Glide Data Grid is canvas-based and touches `window`/`document` at module
 * scope in places, so the grid is loaded client-only (no SSR/prerender).
 */
const GlideSpikeGrid = dynamic(
  () => import("./grid").then((m) => m.GlideSpikeGrid),
  { ssr: false },
);

export default function GlideSpikePage() {
  return (
    <main className="bg-background text-foreground min-h-screen p-6">
      <h1 className="mb-1 text-lg font-semibold">
        Glide Data Grid — React 19 spike
      </h1>
      <p className="text-muted mb-4 text-sm">
        1000 rows, 8 columns, editable text/number/boolean cells, column
        resize, theme bridged from the OKLCH design tokens.
      </p>
      <div className="border-border overflow-hidden rounded-lg border">
        <GlideSpikeGrid />
      </div>
      {/* Glide renders its overlay editors into this portal element. */}
      <div id="portal" style={{ position: "fixed", left: 0, top: 0, zIndex: 9999 }} />
    </main>
  );
}
