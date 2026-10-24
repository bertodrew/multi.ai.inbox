import Link from "next/link";
import { Disclosure } from "./Disclosure";
import { LogoMark, Wordmark } from "./Logo";
import { mainLinks, productLinks } from "./nav";

const chevron = (
  <svg viewBox="0 0 12 12" className="h-3 w-3 transition-transform group-open:rotate-180" aria-hidden="true">
    <path d="M2 4l4 4 4-4" fill="none" stroke="currentColor" strokeWidth="1.5" />
  </svg>
);

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 border-b border-border bg-bg/90 backdrop-blur supports-[backdrop-filter]:bg-bg/75">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-4 focus:z-50 focus:rounded focus:bg-card focus:px-3 focus:py-2"
      >
        Skip to content
      </a>
      <div className="mx-auto flex h-16 w-full max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
        <Link href="/" className="flex items-center gap-2" aria-label="multi.ai.inbox home">
          <LogoMark />
          <Wordmark />
        </Link>

        <nav aria-label="Main" className="hidden items-center gap-1 md:flex">
          <Disclosure
            className="relative"
            summaryClassName="flex min-h-11 items-center gap-1 rounded-md px-3 text-sm text-fg hover:bg-card"
            summary={<>Product {chevron}</>}
          >
            <ul className="absolute top-12 left-0 w-52 rounded-lg border border-border bg-card p-2 shadow-lg">
              {productLinks.map((l) => (
                <li key={l.href}>
                  <Link href={l.href} className="block rounded-md px-3 py-2 text-sm hover:bg-bg">
                    {l.label}
                  </Link>
                </li>
              ))}
            </ul>
          </Disclosure>
          {mainLinks.map((l) => (
            <Link key={l.href} href={l.href} className="flex min-h-11 items-center rounded-md px-3 text-sm hover:bg-card">
              {l.label}
            </Link>
          ))}
          <Link
            href="/login/"
            className="ml-2 inline-flex min-h-10 items-center rounded-lg px-4 text-sm font-medium hover:bg-card"
          >
            Sign in
          </Link>
          <Link
            href="/console/"
            className="inline-flex min-h-10 items-center rounded-lg border border-border px-4 text-sm font-medium hover:border-fg/40"
          >
            Console
          </Link>
          <Link
            href="/docs/quickstart/"
            className="inline-flex min-h-10 items-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg hover:opacity-90"
          >
            Quickstart
          </Link>
        </nav>

        <Disclosure
          className="md:hidden"
          summaryClassName="flex min-h-11 items-center gap-1 rounded-md border border-border px-3 text-sm"
          summary={
            <>
              Menu {chevron}
            </>
          }
        >
          <nav
            aria-label="Mobile"
            className="absolute inset-x-0 top-16 max-h-[calc(100dvh-4rem)] overflow-y-auto border-b border-border bg-bg px-4 pb-6 shadow-lg"
          >
            <p className="mt-4 font-mono text-xs tracking-[0.08em] text-muted uppercase">Product</p>
            <ul className="mt-2 grid grid-cols-2 gap-1">
              {productLinks.map((l) => (
                <li key={l.href}>
                  <Link href={l.href} className="flex min-h-11 items-center rounded-md px-2 text-sm hover:bg-card">
                    {l.label}
                  </Link>
                </li>
              ))}
            </ul>
            <ul className="mt-4 border-t border-border pt-4">
              {mainLinks.map((l) => (
                <li key={l.href}>
                  <Link href={l.href} className="flex min-h-11 items-center rounded-md px-2 text-base hover:bg-card">
                    {l.label}
                  </Link>
                </li>
              ))}
            </ul>
            <Link
              href="/login/"
              className="mt-4 flex min-h-11 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg"
            >
              Sign in
            </Link>
            <Link
              href="/console/"
              className="mt-2 flex min-h-11 items-center justify-center rounded-lg border border-border px-4 text-sm font-medium"
            >
              Console
            </Link>
            <Link
              href="/docs/quickstart/"
              className="mt-2 flex min-h-11 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg"
            >
              Quickstart
            </Link>
          </nav>
        </Disclosure>
      </div>
    </header>
  );
}
