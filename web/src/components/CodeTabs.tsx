"use client";

import { useId, useRef, useState, type KeyboardEvent } from "react";
import type { CodeSample } from "@/content/types";
import { useHydrated } from "@/lib/use-hydrated";
import { CopyButton } from "./CopyButton";

/**
 * Accessible tabs. Without JavaScript (and before hydration) every sample is
 * shown one after the other, so nothing is hidden from no-JS readers.
 */
export function CodeTabs({ samples, label }: { samples: CodeSample[]; label: string }) {
  const hydrated = useHydrated();
  const [active, setActive] = useState(0);
  const baseId = useId();
  const tabs = useRef<Array<HTMLButtonElement | null>>([]);

  if (samples.length === 0) {
    return <p className="text-sm text-muted">No code samples.</p>;
  }

  if (!hydrated) {
    return (
      <div className="my-6 grid gap-4">
        {samples.map((s) => (
          <figure key={s.label} className="overflow-hidden rounded-xl border border-border bg-code-bg text-code-fg">
            <figcaption className="border-b border-white/10 px-4 py-3 font-mono text-xs tracking-[0.08em] text-code-fg/70 uppercase">
              {s.label}
            </figcaption>
            <pre className="overflow-x-auto p-4 text-[13px] leading-relaxed">
              <code className="font-mono">{s.code}</code>
            </pre>
          </figure>
        ))}
      </div>
    );
  }

  const current = samples[active] ?? samples[0];
  if (!current) return null;

  const onKey = (event: KeyboardEvent<HTMLButtonElement>) => {
    let next = active;
    if (event.key === "ArrowRight") next = (active + 1) % samples.length;
    else if (event.key === "ArrowLeft") next = (active - 1 + samples.length) % samples.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = samples.length - 1;
    else return;
    event.preventDefault();
    setActive(next);
    tabs.current[next]?.focus();
  };

  return (
    <div className="my-6 overflow-hidden rounded-xl border border-border bg-code-bg text-code-fg">
      <div className="flex items-center justify-between gap-2 border-b border-white/10 pr-2">
        <div role="tablist" aria-label={label} className="flex overflow-x-auto">
          {samples.map((s, i) => (
            <button
              key={s.label}
              ref={(el) => {
                tabs.current[i] = el;
              }}
              id={`${baseId}-tab-${i}`}
              type="button"
              role="tab"
              aria-selected={i === active}
              aria-controls={`${baseId}-panel`}
              tabIndex={i === active ? 0 : -1}
              onClick={() => setActive(i)}
              onKeyDown={onKey}
              className={`min-h-11 shrink-0 border-b-2 px-4 font-mono text-xs tracking-[0.04em] whitespace-nowrap ${
                i === active ? "border-accent text-code-fg" : "border-transparent text-code-fg/65 hover:text-code-fg"
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <CopyButton text={current.code} className="shrink-0" />
      </div>
      <div role="tabpanel" id={`${baseId}-panel`} aria-labelledby={`${baseId}-tab-${active}`} tabIndex={0}>
        <pre className="overflow-x-auto p-4 text-[13px] leading-relaxed">
          <code className="font-mono">{current.code}</code>
        </pre>
      </div>
    </div>
  );
}
