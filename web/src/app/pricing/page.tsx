import type { Metadata } from "next";
import Link from "next/link";
import { ButtonLink, Container, PageHeader } from "@/components/ui";
import { CONTACT_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Pricing",
  description: "Launch pricing in EUR. EU hosting, webhooks, MCP and the gatekeeper on every plan.",
};

type Plan = {
  name: string;
  price: string;
  period: string;
  yearly?: string;
  blurb: string;
  limits: string[];
  extras: string[];
  planned: string[];
  cta: { label: string; href: string };
  highlight?: boolean;
};

const mail = (subject: string): string => `mailto:${CONTACT_EMAIL}?subject=${encodeURIComponent(subject)}`;

const plans: Plan[] = [
  {
    name: "Free",
    price: "€0",
    period: "per month",
    blurb: "For trying the API and for side projects.",
    limits: ["3 inboxes", "1 custom domain", "100 sends per day", "3,000 sends per month"],
    extras: ["Short \"Sent via multi.ai.inbox\" footer on sent mail"],
    planned: [],
    cta: { label: "Request a staging key", href: mail("Free plan") },
  },
  {
    name: "Builder",
    price: "€15",
    period: "per month",
    yearly: "or €150 per year",
    blurb: "For a product with a few agents in production.",
    limits: ["10 inboxes", "5 custom domains", "1,000 sends per day", "10,000 sends per month"],
    extras: ["No footer"],
    planned: ["One click DNS for supported registrars"],
    cta: { label: "Ask for Builder", href: mail("Builder plan") },
    highlight: true,
  },
  {
    name: "Growth",
    price: "€59",
    period: "per month",
    yearly: "or €590 per year",
    blurb: "For a startup running agents for its customers.",
    limits: ["50 inboxes", "20 custom domains", "2,500 sends per day", "40,000 sends per month"],
    extras: ["No footer"],
    planned: [],
    cta: { label: "Ask for Growth", href: mail("Growth plan") },
  },
  {
    name: "Team",
    price: "€149",
    period: "per month",
    yearly: "or €1,490 per year",
    blurb: "For platforms that give an inbox to each customer.",
    limits: ["150 inboxes", "50 custom domains", "15,000 sends per day", "150,000 sends per month"],
    extras: ["No footer"],
    planned: ["Pods for tenant isolation", "Usage metrics"],
    cta: { label: "Ask for Team", href: mail("Team plan") },
  },
  {
    name: "Enterprise",
    price: "Custom",
    period: "",
    blurb: "For higher volume or specific requirements.",
    limits: ["Custom inboxes", "Custom domains", "Custom send limits"],
    extras: ["Terms agreed in writing"],
    planned: ["Dedicated IP", "SSO", "Self hosting support"],
    cta: { label: "Contact us", href: mail("Enterprise") },
  },
];

const everyPlan = [
  "Data stored in Germany (EU)",
  "REST API and MCP server",
  "Signed webhooks with retries",
  "Gatekeeper prompt injection screening",
  "Threads, labels, attachments, raw messages",
  "Custom domains with DNS verification",
  "Agent to agent delivery",
  "Idempotent sends and replies",
  "Scoped API keys and an audit log",
];

export default function PricingPage() {
  return (
    <>
      <PageHeader
        eyebrow="Pricing"
        title="Simple plans. EU hosting on all of them."
        intro="Prices in EUR, excluding VAT. Yearly billing costs 10 months. The limits below are the ones the API enforces today."
      />
      <Container className="py-12">
        <p className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 font-mono text-xs tracking-[0.08em] uppercase">
          <span className="h-1.5 w-1.5 rounded-full bg-accent" aria-hidden="true" />
          Launch pricing
        </p>
        <ul className="mt-6 grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
          {plans.map((p) => (
            <li
              key={p.name}
              className={`flex flex-col rounded-2xl border bg-card p-6 ${p.highlight ? "border-primary ring-1 ring-primary" : "border-border"}`}
            >
              <h2 className="font-display text-xl font-semibold">{p.name}</h2>
              <p className="mt-1 text-sm text-muted">{p.blurb}</p>
              <p className="mt-6">
                <span className="font-display text-4xl font-semibold">{p.price}</span>
                {p.period ? <span className="ml-1 text-sm text-muted">{p.period}</span> : null}
              </p>
              {p.yearly ? <p className="mt-1 text-sm text-muted">{p.yearly}</p> : null}
              <h3 className="mt-6 font-mono text-xs tracking-[0.08em] text-muted uppercase">Limits</h3>
              <ul className="mt-2 space-y-1.5 text-sm">
                {p.limits.map((l) => (
                  <li key={l} className="flex gap-2">
                    <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-fg" aria-hidden="true" />
                    {l}
                  </li>
                ))}
                {p.extras.map((l) => (
                  <li key={l} className="flex gap-2">
                    <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-fg" aria-hidden="true" />
                    {l}
                  </li>
                ))}
              </ul>
              {p.planned.length > 0 ? (
                <>
                  <h3 className="mt-5 font-mono text-xs tracking-[0.08em] text-muted uppercase">Planned, not yet available</h3>
                  <ul className="mt-2 space-y-1.5 text-sm text-muted">
                    {p.planned.map((l) => (
                      <li key={l}>{l}</li>
                    ))}
                  </ul>
                </>
              ) : null}
              <div className="mt-auto pt-8">
                <ButtonLink href={p.cta.href} variant={p.highlight ? "primary" : "secondary"}>
                  {p.cta.label}
                </ButtonLink>
              </div>
            </li>
          ))}
        </ul>

        <section aria-labelledby="every-plan" className="mt-16 rounded-2xl border border-border bg-card p-6 sm:p-8">
          <h2 id="every-plan" className="font-display text-2xl font-semibold">
            Included in every plan
          </h2>
          <ul className="mt-6 grid gap-x-8 gap-y-2 text-sm sm:grid-cols-2">
            {everyPlan.map((f) => (
              <li key={f} className="flex gap-2">
                <span className="mt-1.5 h-2 w-2 shrink-0 rounded-sm bg-accent" aria-hidden="true" />
                {f}
              </li>
            ))}
          </ul>
        </section>

        <section aria-labelledby="pricing-notes" className="mt-16 max-w-3xl">
          <h2 id="pricing-notes" className="font-display text-2xl font-semibold">
            Good to know
          </h2>
          <dl className="mt-6 space-y-6 text-sm">
            <div>
              <dt className="font-medium">What does launch pricing mean?</dt>
              <dd className="mt-1 text-muted">
                These are the prices we intend to launch with. They can change before general availability; we will
                announce changes before they apply to you.
              </dd>
            </div>
            <div>
              <dt className="font-medium">Can I pay today?</dt>
              <dd className="mt-1 text-muted">
                Not yet. Card payments through Stripe are built but stay switched off while the service is in staging.
                Plans are assigned on request: write to{" "}
                <a className="text-primary underline underline-offset-2" href={`mailto:${CONTACT_EMAIL}`}>
                  {CONTACT_EMAIL}
                </a>
                .
              </dd>
            </div>
            <div>
              <dt className="font-medium">How is VAT handled?</dt>
              <dd className="mt-1 text-muted">
                Prices exclude VAT. At checkout VAT is added for your country, and EU businesses can enter their VAT ID
                for the reverse charge.
              </dd>
            </div>
            <div>
              <dt className="font-medium">What counts as a send?</dt>
              <dd className="mt-1 text-muted">
                Every message sent or replied from your inboxes. Receiving mail does not count. When a limit is
                reached the API answers with <code className="font-mono">limit_exceeded</code>; see{" "}
                <Link className="text-primary underline underline-offset-2" href="/docs/limits/">
                  Limits
                </Link>
                .
              </dd>
            </div>
            <div>
              <dt className="font-medium">Can I use it for cold outreach or newsletters?</dt>
              <dd className="mt-1 text-muted">
                No. Unsolicited bulk mail is not allowed on any plan. See the{" "}
                <Link className="text-primary underline underline-offset-2" href="/terms/">
                  terms
                </Link>
                .
              </dd>
            </div>
          </dl>
        </section>
      </Container>
    </>
  );
}
