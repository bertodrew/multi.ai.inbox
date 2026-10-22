import type { Metadata } from "next";
import Link from "next/link";
import type { ReactNode } from "react";
import { Container, PageHeader } from "@/components/ui";
import { CONTACT_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "FAQ",
  description: "Straight answers about multi.ai.inbox: what it is, EU hosting, limits, MCP, prompt injection, pricing.",
};

const a = (href: string, label: string) => (
  <Link className="text-primary underline underline-offset-2" href={href}>
    {label}
  </Link>
);

const faqs: Array<{ q: string; a: ReactNode }> = [
  {
    q: "What is multi.ai.inbox?",
    a: (
      <>
        An email service built for software agents instead of people. Your code creates inboxes through an API, reads
        incoming mail as parsed threads, gets events as signed webhooks and sends replies. The same operations are
        available as MCP tools. See the {a("/docs/quickstart/", "quickstart")}.
      </>
    ),
  },
  {
    q: "Why not just give my agent a Gmail or Outlook account?",
    a: "Consumer mailboxes are made for humans: accounts are created by hand, automation goes through OAuth on a person's mailbox or through clicking the UI, and running many accounts for bots usually conflicts with their terms. Here an inbox is one API call, and the API returns structured data an agent can use directly.",
  },
  {
    q: "How is this different from a transactional email API?",
    a: "Transactional APIs are very good at sending. They usually do not model a two way inbox: receiving, threading, replies with correct headers, labels for workflow state and per inbox addresses. We focus on the conversation, not on bulk delivery.",
  },
  {
    q: "Where is my data stored?",
    a: "On Hetzner servers in Germany, for every plan. Backups stay in the EU. The only part that leaves Germany is outbound mail to external recipients, which is currently relayed through Brevo in France. Details are in the privacy policy.",
  },
  {
    q: "Can I use it for cold outreach or marketing campaigns?",
    a: (
      <>
        No. Unsolicited bulk mail and cold outreach are not allowed on any plan, because they would damage the sending
        reputation every customer relies on. Accounts that do it are suspended. See the {a("/terms/", "terms")}.
      </>
    ),
  },
  {
    q: "What are the limits?",
    a: (
      <>
        The Free plan allows 3 inboxes, 1 custom domain, 100 sends per day and 3,000 per month. Paid plans raise these
        limits. There are also request and size limits, all listed on the {a("/docs/limits/", "limits page")}.
      </>
    ),
  },
  {
    q: "Can I use my own domain?",
    a: (
      <>
        Yes. Add the domain through the API, publish the MX, SPF, DKIM and DMARC records it returns, and verify. We
        recommend a subdomain such as agents.your-domain.eu. See {a("/docs/custom-domains/", "custom domains")}.
      </>
    ),
  },
  {
    q: "Does it work with Claude Code, Cursor or other MCP clients?",
    a: (
      <>
        Yes. The MCP server speaks streamable HTTP at /mcp and takes the same API key as the REST API. Setup for Claude
        Code and Cursor is in the {a("/docs/mcp/", "MCP docs")}.
      </>
    ),
  },
  {
    q: "How do you deal with prompt injection in emails?",
    a: (
      <>
        Every inbound message is screened by the gatekeeper and gets a verdict, clear or review, with the reasons. Your
        agent should check it before following instructions in an email, and keep sensitive actions behind a
        confirmation. Screening lowers the risk, it does not make untrusted mail safe. See{" "}
        {a("/security/", "security")}.
      </>
    ),
  },
  {
    q: "How do I delete data?",
    a: "Deleting an inbox through the API deletes its threads, messages and attachments. Database backups age out on their rotation schedule. To close your account and delete everything, contact us.",
  },
  {
    q: "How much does it cost?",
    a: (
      <>
        Free is €0, Builder is €15 per month, Team is €149 per month, Enterprise is priced on request. This is launch
        pricing and billing is not live yet. See {a("/pricing/", "pricing")}.
      </>
    ),
  },
  {
    q: "Can I self host it?",
    a: "Not today. The software is proprietary and offered as a hosted service only. If self hosting is a hard requirement for you, tell us: it helps us decide whether to offer it later.",
  },
  {
    q: "Is it production ready?",
    a: `The core API works end to end, but the service is in staging: there is no self service sign up, no billing and no uptime commitment yet. Ask for a staging key at ${CONTACT_EMAIL}.`,
  },
];

export default function FaqPage() {
  return (
    <>
      <PageHeader eyebrow="FAQ" title="Questions, answered plainly" />
      <Container className="py-12">
        <div className="max-w-3xl divide-y divide-border rounded-2xl border border-border bg-card">
          {faqs.map((f) => (
            <details key={f.q} className="group px-5 py-1">
              <summary className="flex min-h-14 cursor-pointer list-none items-center justify-between gap-4 py-3 font-medium [&::-webkit-details-marker]:hidden">
                <h2 className="text-base">{f.q}</h2>
                <span aria-hidden="true" className="shrink-0 font-mono text-lg text-muted transition-transform group-open:rotate-45">
                  +
                </span>
              </summary>
              <div className="pb-5 text-muted">{f.a}</div>
            </details>
          ))}
        </div>
        <p className="mt-8 text-sm text-muted">
          Another question? Write to{" "}
          <a className="text-primary underline underline-offset-2" href={`mailto:${CONTACT_EMAIL}`}>
            {CONTACT_EMAIL}
          </a>
          .
        </p>
      </Container>
    </>
  );
}
