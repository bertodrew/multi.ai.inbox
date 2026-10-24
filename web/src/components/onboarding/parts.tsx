// Presentational pieces of the onboarding funnel: no state, no persistence.
// Sizes are touch first: every control is at least 44px tall, and the layout
// stays single column until there is room for two.

import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost";

const buttonVariants: Record<ButtonVariant, string> = {
  primary: "bg-primary text-primary-fg hover:opacity-90",
  secondary: "border border-border bg-card text-fg hover:border-fg/40",
  ghost: "text-muted hover:bg-card hover:text-fg",
};

export function StepProgress({
  current,
  total,
  labels,
}: {
  current: number;
  total: number;
  labels: readonly string[];
}) {
  const currentLabel = labels.slice(0, current).at(-1) ?? "";

  return (
    <nav aria-label="Setup progress" className="mt-8">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <p className="font-mono text-xs font-medium tracking-[0.08em] text-muted uppercase">
          Step {current} of {total}
        </p>
        <p className="text-sm text-muted">{currentLabel}</p>
      </div>
      <ol className="mt-3 flex list-none gap-2 p-0">
        {labels.map((label, index) => {
          const position = index + 1;
          const reached = position <= current;
          const note = position < current ? "done" : position === current ? "current step" : "not started";
          return (
            <li key={label} className="flex-1">
              <span
                aria-hidden="true"
                className={`block h-1.5 rounded-full ${reached ? "bg-primary" : "bg-border"}`}
              />
              <span className="sr-only">{`${label}: ${note}`}</span>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

export function FlowButton({
  variant = "primary",
  className = "",
  children,
  type = "button",
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant }) {
  return (
    <button
      type={type}
      className={`inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-4 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${buttonVariants[variant]} ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
}

export const inputClass =
  "min-h-11 w-full rounded-lg border border-border bg-bg px-3 py-2 text-base text-fg placeholder:text-muted/70 focus:border-primary disabled:opacity-60 sm:text-sm";

export function TextInput({ className = "", ...rest }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={`${inputClass} ${className}`} {...rest} />;
}

export function Field({
  id,
  label,
  hint,
  error,
  children,
}: {
  id: string;
  label: string;
  hint?: ReactNode;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      {children}
      {hint ? <p className="text-xs text-muted">{hint}</p> : null}
      {error ? (
        <p className="text-sm text-danger" role="alert" id={`${id}-error`}>
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function Panel({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <section className={`rounded-xl border border-border bg-card p-5 sm:p-6 ${className}`}>{children}</section>;
}

export function ChoiceCard({
  name,
  value,
  checked,
  onChange,
  title,
  description,
}: {
  name: string;
  value: string;
  checked: boolean;
  onChange: () => void;
  title: string;
  description: string;
}) {
  return (
    <label
      className={`block cursor-pointer rounded-xl border p-4 transition-colors ${
        checked ? "border-primary bg-bg ring-1 ring-primary" : "border-border bg-bg hover:border-fg/40"
      }`}
    >
      <span className="flex items-start gap-3">
        <input
          type="radio"
          name={name}
          value={value}
          checked={checked}
          onChange={onChange}
          className="mt-1 size-4 shrink-0 accent-primary"
        />
        <span className="min-w-0">
          <span className="block text-sm font-medium">{title}</span>
          <span className="mt-1 block text-sm text-muted">{description}</span>
        </span>
      </span>
    </label>
  );
}

export function Note({ tone = "info", children }: { tone?: "info" | "warn" | "success"; children: ReactNode }) {
  const accent =
    tone === "warn" ? "border-l-accent" : tone === "success" ? "border-l-success" : "border-l-primary";
  return (
    <div className={`rounded-lg border border-border border-l-4 ${accent} bg-card px-4 py-3 text-sm`}>
      <p className="m-0">{children}</p>
    </div>
  );
}

export function EmptyState({
  title,
  children,
  action,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-dashed border-border px-4 py-8 text-center">
      <p className="font-medium">{title}</p>
      {children ? <div className="mt-2 text-sm text-muted">{children}</div> : null}
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
    </div>
  );
}

/** Replaces the funnel while the draft is read from storage, so the markup
 *  rendered on the server and on the first client pass is identical. */
export function FlowSkeleton({ label }: { label: string }) {
  return (
    <div role="status" className="mt-8">
      <div className="h-1.5 w-full animate-pulse rounded-full bg-border" />
      <div className="mt-6 rounded-xl border border-border bg-card p-5 sm:p-6">
        <div className="h-4 w-40 animate-pulse rounded bg-border" />
        <div className="mt-4 h-11 w-full animate-pulse rounded-lg bg-border" />
        <div className="mt-3 h-3 w-2/3 animate-pulse rounded bg-border" />
        <div className="mt-6 h-11 w-32 animate-pulse rounded-lg bg-border" />
      </div>
      <p className="sr-only">{label}</p>
    </div>
  );
}
