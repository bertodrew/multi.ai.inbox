import type { Metadata } from "next";
import Link from "next/link";
import { Container, PageHeader } from "@/components/ui";
import { SECURITY_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Security",
  description:
    "How multi.ai.inbox protects agents and data: prompt injection screening, tenant isolation, hashed keys, signed webhooks, EU hosting.",
};

type Item = { id: string; title: string; points: string[] };

const items: Item[] = [
  {
    id: "gatekeeper",
    title: "Gatekeeper: prompt injection screening",
    points: [
      "Every inbound message is screened before an agent should act on it. The result is stored on the message as a verdict, clear or review, with a score and the reasons.",
      "The rules are transparent: instruction overrides (\"ignore previous instructions\"), role hijacking, requests to send credentials or forward all mail, fake tool or system markup, text hidden with CSS, invisible Unicode characters, and a failing DMARC result.",
      "Screen before notify is on by default: inbound mail is held until the verdict exists, so message.received never reaches your agent before screening finishes.",
      "Messages with verdict review get the review label, stay hidden from every list and read, and fire only the message.security.review event, without the body. A human with a release permission decides; each release is audited.",
      "When the screener is down or slow, the hold fails closed: the message waits for review instead of being delivered unscreened.",
      "The screener also extracts the new text of a reply without quoted history, which shrinks the attack surface the model reads.",
      "Screening reduces risk; it does not remove it. Keep sensitive actions behind a confirmation step in your agent.",
    ],
  },
  {
    id: "isolation",
    title: "Tenant isolation with row level security",
    points: [
      "Every tenant table carries the organization id and has Postgres row level security enabled and forced.",
      "The application connects with a role that is not a superuser, so the policies apply to every query, including bugs.",
      "The end to end test suite tries to read another organization's data and must fail.",
    ],
  },
  {
    id: "keys",
    title: "API keys, permissions and audit log",
    points: [
      "Keys are random, shown once, and stored only as SHA-256 hashes.",
      "Keys can be scoped to a list of permissions and given an expiry date. A key can never grant permissions it does not hold, and revocation is immediate.",
      "MCP tools check the same permissions as the REST endpoints, so a scoped key also limits what a coding agent can do.",
      "Security relevant actions, including agent actions through MCP, are written to an audit log you can read through the API.",
      "The fixed mai_live_ prefix makes leaked keys easy to detect with secret scanners.",
      "Repeated failed authentications from the same address are throttled.",
    ],
  },
  {
    id: "webhooks",
    title: "Signed webhooks and SSRF protection",
    points: [
      "Every delivery is signed with HMAC SHA-256 following the Standard Webhooks scheme, with a per webhook whsec_ secret and a timestamp checked within 5 minutes.",
      "Webhook URLs must use HTTPS. Connections to private, loopback, link local and unspecified addresses are refused when the socket opens, after DNS resolution, so DNS tricks do not reach internal services.",
      "Redirects are not followed and each attempt times out after 15 seconds.",
    ],
  },
  {
    id: "email-auth",
    title: "Email authentication",
    points: [
      "Outbound mail is DKIM signed. Each custom domain gets its own key; SPF and DMARC records are part of domain verification.",
      "Inbound mail is checked for a DKIM signature aligned with the From domain, and the result is exposed on every message.",
      "Inbound mail is scored by a spam filter. Clear spam is rejected during the SMTP session; suspected spam is kept with the spam label.",
      "Mail to unknown recipients is refused at the SMTP RCPT stage, so the platform does not accept mail it cannot deliver.",
    ],
  },
  {
    id: "abuse",
    title: "Abuse prevention",
    points: [
      "Daily and monthly send limits per plan, a footer on the free plan, and a cap of 50 recipients per message.",
      "Cold outreach and unsolicited bulk mail are forbidden by the terms. Accounts that send it are suspended.",
      "Per organization API rate limits.",
    ],
  },
  {
    id: "eu",
    title: "EU hosting",
    points: [
      "Application servers, the database and stored mail run on Hetzner Online GmbH infrastructure in Germany.",
      "Nightly database backups go to Hetzner Object Storage in the EU.",
      "Outbound mail to external recipients is currently relayed by Brevo (France). Mail between inboxes on the platform does not leave it.",
      "The API is served over TLS with certificates from Let's Encrypt, and the SMTP server offers STARTTLS.",
    ],
  },
];

export default function SecurityPage() {
  return (
    <>
      <PageHeader
        eyebrow="Security"
        title="Built for untrusted input"
        intro="Email is the easiest way to reach an agent, which makes it the easiest way to attack one. This page describes the controls that exist in the product today."
      />
      <Container className="py-12">
        <div className="grid gap-12 lg:grid-cols-[14rem_minmax(0,1fr)]">
          <nav aria-label="On this page" className="hidden lg:block">
            <ul className="sticky top-24 space-y-1 text-sm">
              {items.map((i) => (
                <li key={i.id}>
                  <a href={`#${i.id}`} className="block rounded-md px-2 py-1.5 text-muted hover:bg-card hover:text-fg">
                    {i.title}
                  </a>
                </li>
              ))}
              <li>
                <a href="#compliance" className="block rounded-md px-2 py-1.5 text-muted hover:bg-card hover:text-fg">
                  Compliance status
                </a>
              </li>
              <li>
                <a href="#disclosure" className="block rounded-md px-2 py-1.5 text-muted hover:bg-card hover:text-fg">
                  Responsible disclosure
                </a>
              </li>
            </ul>
          </nav>
          <div className="min-w-0 max-w-3xl">
            {items.map((i) => (
              <section key={i.id} id={i.id} aria-labelledby={`${i.id}-h`} className="mb-12">
                <h2 id={`${i.id}-h`} className="font-display text-2xl font-semibold">
                  {i.title}
                </h2>
                <ul className="mt-4 space-y-3">
                  {i.points.map((p) => (
                    <li key={p} className="flex gap-3">
                      <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-accent" aria-hidden="true" />
                      <span>{p}</span>
                    </li>
                  ))}
                </ul>
              </section>
            ))}

            <section id="compliance" aria-labelledby="compliance-h" className="mb-12 rounded-xl border border-border bg-card p-6">
              <h2 id="compliance-h" className="font-display text-2xl font-semibold">
                Compliance status
              </h2>
              <p className="mt-3">
                We do not hold SOC 2, ISO 27001 or any other certification today, and we will not claim one until an
                audit is complete. The service is in staging and offers no uptime commitment yet. Data protection is
                described in the{" "}
                <Link className="text-primary underline underline-offset-2" href="/privacy/">
                  privacy policy
                </Link>{" "}
                (draft).
              </p>
            </section>

            <section id="disclosure" aria-labelledby="disclosure-h">
              <h2 id="disclosure-h" className="font-display text-2xl font-semibold">
                Responsible disclosure
              </h2>
              <p className="mt-3">
                If you find a vulnerability, write to{" "}
                <a className="text-primary underline underline-offset-2" href={`mailto:${SECURITY_EMAIL}`}>
                  {SECURITY_EMAIL}
                </a>{" "}
                with the steps to reproduce it. Please:
              </p>
              <ul className="mt-3 space-y-2">
                <li className="flex gap-3">
                  <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-fg" aria-hidden="true" />
                  test only against accounts and data you own;
                </li>
                <li className="flex gap-3">
                  <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-fg" aria-hidden="true" />
                  do not send spam, run denial of service tests or access other customers&apos; mail;
                </li>
                <li className="flex gap-3">
                  <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-fg" aria-hidden="true" />
                  give us reasonable time to fix the issue before you publish it.
                </li>
              </ul>
              <p className="mt-3">
                We will acknowledge your report, keep you informed and credit you if you wish. We do not run a paid bug
                bounty yet.
              </p>
            </section>
          </div>
        </div>
      </Container>
    </>
  );
}
