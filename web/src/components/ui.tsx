import Link from "next/link";
import type { ReactNode } from "react";

export function Container({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`mx-auto w-full max-w-6xl px-4 sm:px-6 ${className}`}>{children}</div>;
}

export function Eyebrow({ children, as = "p" }: { children: ReactNode; as?: "p" | "span" }) {
  const Tag = as;
  return <Tag className="font-mono text-xs font-medium tracking-[0.08em] text-muted uppercase">{children}</Tag>;
}

type ButtonProps = {
  href: string;
  children: ReactNode;
  variant?: "primary" | "secondary";
};

const buttonBase =
  "inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium transition-colors";

export function ButtonLink({ href, children, variant = "primary" }: ButtonProps) {
  const style =
    variant === "primary"
      ? "bg-primary text-primary-fg hover:opacity-90"
      : "border border-border bg-card text-fg hover:border-fg/40";
  const external = href.startsWith("mailto:") || href.startsWith("http");
  if (external) {
    return (
      <a href={href} className={`${buttonBase} ${style}`}>
        {children}
      </a>
    );
  }
  return (
    <Link href={href} className={`${buttonBase} ${style}`}>
      {children}
    </Link>
  );
}

export function SectionHeading({
  eyebrow,
  title,
  intro,
  id,
}: {
  eyebrow: string;
  title: string;
  intro?: ReactNode;
  id?: string;
}) {
  return (
    <div className="max-w-2xl">
      <Eyebrow>{eyebrow}</Eyebrow>
      <h2 id={id} className="mt-3 font-display text-3xl font-semibold tracking-tight sm:text-4xl">
        {title}
      </h2>
      {intro ? <p className="mt-4 text-lg text-muted">{intro}</p> : null}
    </div>
  );
}

export function PageHeader({ eyebrow, title, intro }: { eyebrow: string; title: string; intro?: ReactNode }) {
  return (
    <header className="border-b border-border py-12 sm:py-16">
      <Container>
        <Eyebrow>{eyebrow}</Eyebrow>
        <h1 className="mt-3 max-w-3xl font-display text-4xl font-semibold tracking-tight sm:text-5xl">{title}</h1>
        {intro ? <p className="mt-4 max-w-2xl text-lg text-muted">{intro}</p> : null}
      </Container>
    </header>
  );
}

export function Note({ tone = "info", children }: { tone?: "info" | "warn"; children: ReactNode }) {
  const bar = tone === "warn" ? "border-l-accent" : "border-l-primary";
  return (
    <aside className={`my-6 rounded-lg border border-border border-l-4 ${bar} bg-card px-4 py-3 text-sm`}>
      <p className="m-0">
        <span className="mr-2 font-mono text-xs font-medium tracking-[0.08em] uppercase">
          {tone === "warn" ? "Careful" : "Note"}
        </span>
        {children}
      </p>
    </aside>
  );
}

export function DraftNotice() {
  return (
    <div role="note" className="rounded-lg border border-border border-l-4 border-l-accent bg-card px-4 py-3 text-sm">
      <strong className="font-semibold">Draft, to be reviewed by counsel before launch.</strong> This text is not
      final and is not legal advice. Placeholders in square brackets will be completed before launch.
    </div>
  );
}
