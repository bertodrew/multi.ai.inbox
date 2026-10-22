import Link from "next/link";
import { CONTACT_EMAIL, STATUS_NOTE } from "@/lib/site";
import { LogoMark, Wordmark } from "./Logo";
import { footerLinks } from "./nav";

export function SiteFooter() {
  return (
    <footer className="mt-24 border-t border-border">
      <div className="mx-auto grid w-full max-w-6xl gap-10 px-4 py-12 sm:px-6 md:grid-cols-[1fr_auto]">
        <div>
          <Link href="/" className="inline-flex items-center gap-2" aria-label="multi.ai.inbox home">
            <LogoMark className="h-6 w-6" />
            <Wordmark />
          </Link>
          <p className="mt-3 max-w-sm text-sm text-muted">
            Inboxes for AI agents. Hosted in Europe, on servers in Germany.
          </p>
          <p className="mt-3 text-sm">
            Contact:{" "}
            <a className="text-primary underline underline-offset-2" href={`mailto:${CONTACT_EMAIL}`}>
              {CONTACT_EMAIL}
            </a>
          </p>
        </div>
        <nav aria-label="Footer">
          <ul className="grid grid-cols-2 gap-x-10 gap-y-1 sm:grid-cols-4 md:grid-cols-2">
            {footerLinks.map((l) => (
              <li key={l.href}>
                <Link href={l.href} className="inline-flex min-h-10 items-center text-sm text-muted hover:text-fg">
                  {l.label}
                </Link>
              </li>
            ))}
          </ul>
        </nav>
      </div>
      <div className="border-t border-border">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-3 px-4 py-5 text-xs text-muted sm:px-6">
          <p>
            <span className="mr-2 inline-flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5 font-mono tracking-[0.08em] uppercase">
              <span className="h-1.5 w-1.5 rounded-full bg-accent" aria-hidden="true" />
              {STATUS_NOTE}
            </span>
            The service is in staging: no uptime commitment yet.
          </p>
          <p>Proprietary software. No cookies, no analytics on this site.</p>
        </div>
      </div>
    </footer>
  );
}
