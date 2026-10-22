import type { CodeSample } from "@/content/types";
import { CopyButton } from "./CopyButton";

export function CodeBlock({ sample, showLabel = true }: { sample: CodeSample; showLabel?: boolean }) {
  return (
    <figure className="my-6 overflow-hidden rounded-xl border border-border bg-code-bg text-code-fg">
      <figcaption className="flex min-h-11 items-center justify-between gap-3 border-b border-white/10 px-4 py-1">
        <span className="font-mono text-xs tracking-[0.08em] text-code-fg/70 uppercase">
          {showLabel ? sample.label : sample.lang}
        </span>
        <CopyButton text={sample.code} />
      </figcaption>
      <pre className="overflow-x-auto p-4 text-[13px] leading-relaxed">
        <code className="font-mono">{sample.code}</code>
      </pre>
    </figure>
  );
}
