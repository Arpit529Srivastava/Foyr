"use client";

import { useState, type ReactNode } from "react";
import { Check, Copy } from "lucide-react";

export type CodeSample = { id: string; label: string; file: string; source: string; lines: ReactNode };

/** Framework-tabbed code window with a copy button. `lines` is the highlighted rendering of `source`. */
export function CodeTabs({ samples, minHeight = 312 }: { samples: CodeSample[]; minHeight?: number }) {
  const [active, setActive] = useState(samples[0].id);
  const [copied, setCopied] = useState(false);
  const sample = samples.find((s) => s.id === active)!;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(sample.source);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard blocked: nothing to do */
    }
  };

  return (
    <div className="overflow-hidden rounded-xl border border-line bg-panel">
      <div className="flex h-12 items-center justify-between gap-3 border-b border-line px-2.5">
        <div role="tablist" aria-label="Framework" className="flex gap-1">
          {samples.map((s) => (
            <button
              key={s.id}
              type="button"
              role="tab"
              aria-selected={s.id === active}
              onClick={() => setActive(s.id)}
              className={`h-[30px] cursor-pointer rounded-md px-2.5 text-[13px] font-medium transition-colors ${
                s.id === active ? "bg-raised text-white" : "text-subtle hover:text-fg"
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <div className="flex min-w-0 items-center gap-3">
          <span className="hidden truncate font-mono text-xs text-subtle sm:inline">{sample.file}</span>
          <button
            type="button"
            aria-label={copied ? "Copied" : "Copy code"}
            onClick={copy}
            className="flex size-[30px] shrink-0 cursor-pointer items-center justify-center rounded-md border border-line bg-card text-dim transition-colors hover:text-white"
          >
            {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
          </button>
        </div>
      </div>
      <div
        role="tabpanel"
        className="overflow-x-auto px-6 py-[22px] font-mono text-[13px] leading-[22px] text-fg sm:text-sm"
        style={{ minHeight }}
      >
        <pre key={sample.id} className="animate-fade font-mono">
          <code>{sample.lines}</code>
        </pre>
      </div>
    </div>
  );
}

/* Tiny token helpers matching the design's monochrome + amber highlighting. */
export const K = ({ children }: { children: ReactNode }) => <span className="text-subtle">{children}</span>;
export const F = ({ children }: { children: ReactNode }) => <span className="text-amber">{children}</span>;
export const S = ({ children }: { children: ReactNode }) => <span className="text-dim">{children}</span>;
export const C = ({ children }: { children: ReactNode }) => <span className="text-subtle italic">{children}</span>;
