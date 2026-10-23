/** P.O. box mark: a 2 by 2 grid of mail slots, the top right one has mail. */
export function LogoMark({ className = "h-7 w-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden="true" focusable="false">
      <rect x="1" y="1" width="30" height="30" rx="7" fill="var(--fg)" />
      <rect x="6" y="6" width="9" height="9" rx="2" fill="var(--bg)" />
      <rect x="17" y="6" width="9" height="9" rx="2" fill="var(--accent)" />
      <rect x="6" y="17" width="9" height="9" rx="2" fill="var(--bg)" />
      <rect x="17" y="17" width="9" height="9" rx="2" fill="var(--bg)" />
    </svg>
  );
}

export function Wordmark() {
  return (
    <span className="font-display text-lg font-semibold tracking-tight">
      multi<span className="text-accent">.</span>ai<span className="text-accent">.</span>inbox
      <sup className="ml-0.5 align-super text-[0.55em] font-normal">™</sup>
    </span>
  );
}
