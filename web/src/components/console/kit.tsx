"use client";

// Small building blocks shared by the console pages: buttons, fields,
// cards, and the loading, error and empty states every view needs.
import { useEffect, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from "react";
import { ApiError } from "@/lib/console/client";
import { useToast } from "./Toast";

type Variant = "primary" | "secondary" | "danger" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-primary text-primary-fg hover:opacity-90",
  secondary: "border border-border bg-card text-fg hover:border-fg/40",
  danger: "border border-danger/40 bg-card text-danger hover:bg-danger hover:text-white",
  ghost: "text-fg hover:bg-card",
};

export function Button({
  variant = "secondary",
  busy = false,
  className = "",
  children,
  disabled,
  type = "button",
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; busy?: boolean }) {
  return (
    <button
      type={type}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={`inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-4 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${variants[variant]} ${className}`}
      {...rest}
    >
      {busy ? <Spinner /> : null}
      {children}
    </button>
  );
}

export function Spinner({ className = "h-4 w-4" }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={`animate-spin ${className}`} aria-hidden="true">
      <circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeOpacity="0.25" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <section className={`rounded-xl border border-border bg-card p-4 sm:p-6 ${className}`}>{children}</section>;
}

export function PageTitle({ title, intro, actions }: { title: string; intro?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        <h1 className="font-display text-2xl font-semibold tracking-tight break-words sm:text-3xl">{title}</h1>
        {intro ? <p className="mt-2 max-w-2xl text-muted">{intro}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </div>
  );
}

export function Field({
  label,
  hint,
  error,
  htmlFor,
  children,
}: {
  label: string;
  hint?: ReactNode;
  error?: string;
  htmlFor: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={htmlFor} className="text-sm font-medium">
        {label}
      </label>
      {children}
      {hint ? <p className="text-xs text-muted">{hint}</p> : null}
      {error ? (
        <p className="text-sm text-danger" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export const inputClass =
  "min-h-11 w-full rounded-lg border border-border bg-bg px-3 py-2 text-base text-fg placeholder:text-muted/70 focus:border-primary sm:text-sm";

export function LoadingState({ label }: { label: string }) {
  return (
    <div role="status" className="flex items-center justify-center gap-3 rounded-xl border border-dashed border-border px-4 py-12 text-muted">
      <Spinner />
      <span>{label}</span>
    </div>
  );
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="rounded-xl border border-dashed border-border px-4 py-12 text-center">
      <p className="font-medium">{title}</p>
      {children ? <div className="mt-2 text-sm text-muted">{children}</div> : null}
    </div>
  );
}

/** Error state with a clear explanation for permission and sign in problems. */
export function ErrorState({ error, onRetry, what }: { error: unknown; onRetry?: () => void; what: string }) {
  let title = `Could not load ${what}`;
  let body: ReactNode = error instanceof Error ? error.message : "Something went wrong.";
  if (error instanceof ApiError && error.isPermission) {
    title = "This API key lacks a permission";
    body = (
      <>
        {error.message}. Sign in with a key that holds it, or ask an owner of a full access key to create one for you
        on the API keys page.
      </>
    );
  } else if (error instanceof ApiError && error.isAuth) {
    title = "The API key was rejected";
    body = <>{error.message}. It may have been revoked or have expired; sign out and sign in with a valid key.</>;
  }
  const requestId = error instanceof ApiError ? error.requestId : "";
  return (
    <div role="alert" className="rounded-xl border border-border border-l-4 border-l-danger bg-card px-4 py-5">
      <p className="font-medium">{title}</p>
      <p className="mt-1 text-sm break-words text-muted">{body}</p>
      {requestId ? <p className="mt-2 font-mono text-xs text-muted">Request {requestId}</p> : null}
      {onRetry ? (
        <Button className="mt-4" onClick={onRetry}>
          Try again
        </Button>
      ) : null}
    </div>
  );
}

/**
 * A button that asks before a destructive action. The dialog stays open
 * while the action runs and closes once it resolves to true.
 */
export function ConfirmButton({
  label,
  title,
  body,
  confirmLabel,
  onConfirm,
  busy = false,
  className = "",
}: {
  label: ReactNode;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  onConfirm: () => Promise<boolean>;
  busy?: boolean;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  const confirm = async () => {
    const done = await onConfirm();
    if (done) setOpen(false);
  };

  return (
    <>
      <Button variant="danger" className={className} onClick={() => setOpen(true)} busy={busy}>
        {label}
      </Button>
      <dialog
        ref={ref}
        onClose={() => setOpen(false)}
        className="m-auto w-[calc(100%-2rem)] max-w-md rounded-xl border border-border bg-card p-0 text-fg backdrop:bg-black/50"
      >
        {open ? (
          <div className="p-5">
            <h2 className="font-display text-lg font-semibold">{title}</h2>
            <div className="mt-2 text-sm break-words text-muted">{body}</div>
            <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button onClick={() => setOpen(false)} disabled={busy}>
                Cancel
              </Button>
              <Button variant="danger" onClick={confirm} busy={busy} autoFocus>
                {confirmLabel}
              </Button>
            </div>
          </div>
        ) : null}
      </dialog>
    </>
  );
}

export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: "neutral" | "good" | "warn" | "bad" }) {
  const tones = {
    neutral: "border-border text-muted",
    good: "border-success/40 text-success",
    warn: "border-accent/60 text-fg",
    bad: "border-danger/40 text-danger",
  };
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 font-mono text-xs ${tones[tone]}`}>
      {children}
    </span>
  );
}

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

export function When({ iso }: { iso: string | null | undefined }) {
  if (!iso) return <span className="text-muted">never</span>;
  const d = new Date(iso);
  return (
    <time dateTime={iso} title={iso}>
      {Number.isNaN(d.getTime()) ? iso : dateFormat.format(d)}
    </time>
  );
}

/** Shows a secret once with a copy button and a clear warning. */
export function SecretOnce({ label, value, onDone }: { label: string; value: string; onDone: () => void }) {
  const toast = useToast();
  const [copied, setCopied] = useState<"idle" | "ok" | "failed">("idle");
  const copy = async () => {
    try {
      if (!navigator.clipboard) throw new Error("The clipboard is not available in this browser.");
      await navigator.clipboard.writeText(value);
      setCopied("ok");
      toast.success("Copied to the clipboard");
    } catch (err) {
      setCopied("failed");
      toast.error("Copy failed", err instanceof Error ? err.message : "Select the text and copy it.");
    }
  };
  return (
    <div role="alert" className="rounded-xl border border-border border-l-4 border-l-accent bg-card p-4 sm:p-5">
      <p className="font-medium">{label}</p>
      <p className="mt-1 text-sm text-muted">
        Copy it now and store it in a secret manager. It is shown only this once and cannot be retrieved later.
      </p>
      <code className="mt-3 block overflow-x-auto rounded-lg bg-code-bg px-3 py-3 font-mono text-sm break-all whitespace-pre-wrap text-code-fg">
        {value}
      </code>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button variant="primary" onClick={copy}>
          {copied === "ok" ? "Copied" : "Copy"}
        </Button>
        <Button onClick={onDone}>I stored it, hide it</Button>
        {copied === "failed" ? <span className="text-sm text-danger">Copy failed: select the text and copy it.</span> : null}
      </div>
    </div>
  );
}
