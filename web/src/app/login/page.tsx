import type { Metadata } from "next";
import Link from "next/link";
import { ButtonLink, Container, Eyebrow } from "@/components/ui";
import { SessionFromHash } from "@/components/auth/SessionFromHash";

export const metadata: Metadata = {
  title: "Sign in",
  description: "Sign in to the multi.ai.inbox console with Google, GitHub or a single use link sent by email.",
  robots: { index: false, follow: false },
};

/** Where a browser starts an OAuth flow; the API redirects to the provider. */
const oauth = (provider: "google" | "github"): string => `/v0/auth/oauth/${provider}/start?redirect=1`;

export default function LoginPage() {
  return (
    <>
      <SessionFromHash />
    <Container className="py-16 sm:py-24">
      <div className="mx-auto w-full max-w-md">
        <Eyebrow>Console</Eyebrow>
        <h1 className="mt-3 font-display text-3xl font-semibold tracking-tight sm:text-4xl">Sign in</h1>
        <p className="mt-4 text-muted">
          One account for your organization. Agents keep using their API keys and never sign in here.
        </p>

        <div className="mt-8 rounded-xl border border-border bg-card p-6">
          <div className="flex flex-col gap-3">
            <ButtonLink href={oauth("google")} variant="secondary">
              Continue with Google
            </ButtonLink>
            <ButtonLink href={oauth("github")} variant="secondary">
              Continue with GitHub
            </ButtonLink>
          </div>

          <div className="my-6 flex items-center gap-3">
            <span className="h-px flex-1 bg-border" aria-hidden="true" />
            <span className="font-mono text-xs tracking-[0.08em] text-muted uppercase">or</span>
            <span className="h-px flex-1 bg-border" aria-hidden="true" />
          </div>

          <form action="/v0/auth/magic-link" method="post" className="flex flex-col gap-3">
            <input type="hidden" name="redirect_after" value="/login/" />
            <label htmlFor="email" className="text-sm font-medium">
              Work email
            </label>
            <input
              id="email"
              name="email"
              type="email"
              required
              autoComplete="email"
              placeholder="you@company.com"
              className="min-h-11 rounded-lg border border-border bg-bg px-3 text-sm text-fg outline-none focus-visible:border-fg/40"
            />
            <button
              type="submit"
              className="inline-flex min-h-11 items-center justify-center rounded-lg bg-primary px-5 py-2.5 text-sm font-medium text-primary-fg transition-colors hover:opacity-90"
            >
              Email me a sign in link
            </button>
          </form>

          <p className="mt-4 text-sm text-muted">
            The link works once and expires in 15 minutes. If it does not arrive in a minute, check the spam folder
            before asking for another.
          </p>
        </div>

        <p className="mt-6 text-sm text-muted">
          Agents authenticate with an API key instead: see the{" "}
          <Link href="/docs/quickstart/" className="underline underline-offset-4 hover:text-fg">
            quickstart
          </Link>
          . Sessions last 15 minutes and renew automatically while the console is open.
        </p>
      </div>
    </Container>
    </>
  );
}
