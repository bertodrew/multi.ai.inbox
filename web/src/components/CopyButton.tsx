"use client";

import { useEffect, useRef, useState } from "react";
import { useHydrated } from "@/lib/use-hydrated";

type State = "idle" | "copying" | "copied" | "error";

const labels: Record<State, string> = {
  idle: "Copy",
  copying: "Copying",
  copied: "Copied",
  error: "Copy failed, select the text",
};

/** Renders only once JavaScript runs, so it never shows as a dead button. */
export function CopyButton({ text, label = "Copy", className = "" }: { text: string; label?: string; className?: string }) {
  const hydrated = useHydrated();
  const [state, setState] = useState<State>("idle");
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  if (!hydrated) return null;

  const copy = async () => {
    setState("copying");
    try {
      if (!navigator.clipboard) throw new Error("clipboard unavailable");
      await navigator.clipboard.writeText(text);
      setState("copied");
    } catch {
      setState("error");
    }
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setState("idle"), 2500);
  };

  return (
    <button
      type="button"
      onClick={copy}
      disabled={state === "copying"}
      className={`inline-flex min-h-9 items-center rounded-md border border-white/20 px-3 font-mono text-xs text-code-fg hover:border-white/50 disabled:opacity-60 ${className}`}
    >
      <span aria-live="polite">{state === "idle" ? label : labels[state]}</span>
    </button>
  );
}
