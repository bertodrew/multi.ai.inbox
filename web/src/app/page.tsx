import Link from "next/link";
import { CodeTabs } from "@/components/CodeTabs";
import { CopyButton } from "@/components/CopyButton";
import { PoBoxWall } from "@/components/PoBoxWall";
import { ButtonLink, Container, Eyebrow, SectionHeading } from "@/components/ui";
import { agentPrompt, homeSamples } from "@/content/snippets";
import { CONTACT_EMAIL, SITE_URL } from "@/lib/site";

const steps = [
  {
    n: "01",
    title: "Create an inbox",
    body: "One POST returns a working address on the shared domain or on your own verified domain. Repeat the call with the same client_id and you get the same inbox back.",
    code: "POST /v0/inboxes",
  },
  {
    n: "02",
    title: "Receive as threads",
    body: "Inbound mail is parsed, threaded by its headers and screened for prompt injection. Your agent reads extracted_text without quoted history.",
    code: "GET /v0/inboxes/{inbox}/threads",
  },
  {
    n: "03",
    title: "Get signed webhooks",
    body: "Events such as message.received reach your endpoint signed with the Standard Webhooks scheme, retried with backoff when your endpoint is down.",
    code: "POST /v0/webhooks",
  },
  {
    n: "04",
    title: "Reply in thread",
    body: "Replies keep In-Reply-To and References right and are DKIM signed. An Idempotency-Key makes retries safe: the same key never sends twice.",
    code: "POST .../messages/{message}/reply",
  },
];

const features = [
  { title: "Inboxes by API", body: "Create, list and delete inboxes in code. One per agent, per customer or per task." },
  { title: "Threads, not MIME", body: "Parsed bodies, attachments, labels and extracted_text, grouped into conversations." },
  { title: "Signed webhooks", body: "Standard Webhooks headers plus svix duplicates, HMAC SHA-256, 5 minute tolerance." },
  { title: "Gatekeeper", body: "Every inbound message gets a prompt injection verdict, clear or review, with reasons." },
  { title: "MCP server", body: "Eleven tools at /mcp for Claude Code, Cursor and Codex, with the same API key and permissions." },
  { title: "Custom domains", body: "Add a domain, publish MX, SPF, DKIM and DMARC, verify. A new DKIM key per domain." },
  { title: "Agent to agent", body: "Mail between inboxes on the platform is delivered locally and never leaves the system." },
  { title: "Idempotent sends", body: "Idempotency-Key on send and reply, client_id on inbox creation. Retries are safe." },
  { title: "Raw access", body: "Download the original RFC 822 message and see DKIM and spam results for every message." },
];

const securityPoints = [
  { title: "Tenant isolation", body: "Row level security on every table, enforced by the database, tested across organizations." },
  { title: "Hashed API keys", body: "Keys are shown once and stored as SHA-256 hashes, with a prefix for secret scanning." },
  { title: "Prompt injection screening", body: "Transparent rules flag instruction overrides, exfiltration attempts and hidden text." },
  { title: "Safe webhooks", body: "HTTPS only, private addresses refused at connect time, redirects not followed." },
];

export default function Home() {
  const prompt = agentPrompt(SITE_URL);
  return (
    <>
      <section aria-labelledby="hero-title" className="border-b border-border">
        <Container className="grid items-center gap-12 py-16 sm:py-24 lg:grid-cols-[1.1fr_1fr]">
          <div>
            <Eyebrow>Email API for AI agents</Eyebrow>
            <h1
              id="hero-title"
              className="mt-4 font-display text-4xl leading-[1.05] font-semibold tracking-tight text-balance sm:text-5xl lg:text-[56px]"
            >
              Inboxes for AI agents. Hosted in Europe.
            </h1>
            <p className="mt-6 max-w-xl text-lg text-muted">
              Give every agent its own email address with one API call. Mail arrives as parsed threads, events
              arrive as signed webhooks, and the data is stored on servers in Germany.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <ButtonLink href="/docs/quickstart/">Read the quickstart</ButtonLink>
              <ButtonLink href="/pricing/" variant="secondary">
                See pricing
              </ButtonLink>
            </div>
            <ul className="mt-8 flex flex-wrap gap-x-5 gap-y-2 font-mono text-xs text-muted">
              <li>REST /v0</li>
              <li>MCP /mcp</li>
              <li>Signed webhooks</li>
              <li>Stored in Germany</li>
            </ul>
          </div>
          <div className="flex justify-center lg:justify-end">
            <PoBoxWall />
          </div>
        </Container>
      </section>

      <section id="how-it-works" aria-labelledby="how-title" className="py-20">
        <Container>
          <SectionHeading
            id="how-title"
            eyebrow="How it works"
            title="An inbox is a row. Mail is an event."
            intro="No mail server to run, no human mailbox to borrow. Four calls cover the whole loop."
          />
          <ol className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {steps.map((s) => (
              <li key={s.n} className="flex flex-col rounded-xl border border-border bg-card p-5">
                <span className="font-mono text-xs text-muted">{s.n}</span>
                <h3 className="mt-3 font-display text-lg font-semibold">{s.title}</h3>
                <p className="mt-2 flex-1 text-sm text-muted">{s.body}</p>
                <code className="mt-4 block rounded-md bg-bg px-2 py-1.5 font-mono text-xs break-all">{s.code}</code>
              </li>
            ))}
          </ol>
        </Container>
      </section>

      <section id="code" aria-labelledby="code-title" className="border-y border-border bg-card/50 py-20">
        <Container className="grid gap-10 lg:grid-cols-[1fr_1.6fr]">
          <div>
            <SectionHeading
              id="code-title"
              eyebrow="Code first"
              title="Plain HTTP. Any language."
              intro="No SDK required: the API is JSON over HTTPS with a bearer key. Or connect your coding agent to the MCP server and let it use the tools directly."
            />
            <p className="mt-6 text-sm">
              <Link className="text-primary underline underline-offset-2" href="/docs/api-reference/">
                Full API reference
              </Link>
            </p>
          </div>
          <div className="min-w-0">
            <CodeTabs samples={homeSamples} label="Quickstart code" />
          </div>
        </Container>
      </section>

      <section id="features" aria-labelledby="features-title" className="py-20">
        <Container>
          <SectionHeading id="features-title" eyebrow="Features" title="What ships today" />
          <ul className="mt-12 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 lg:grid-cols-3">
            {features.map((f) => (
              <li key={f.title} className="bg-card p-6">
                <h3 className="font-display text-base font-semibold">
                  <span className="mr-2 inline-block h-2 w-2 rounded-sm bg-accent align-middle" aria-hidden="true" />
                  {f.title}
                </h3>
                <p className="mt-2 text-sm text-muted">{f.body}</p>
              </li>
            ))}
          </ul>
        </Container>
      </section>

      <section id="security" aria-labelledby="security-title" className="border-y border-border bg-code-bg py-20 text-code-fg">
        <Container>
          <div className="max-w-2xl">
            <p className="font-mono text-xs font-medium tracking-[0.08em] text-code-fg/70 uppercase">Security</p>
            <h2 id="security-title" className="mt-3 font-display text-3xl font-semibold tracking-tight sm:text-4xl">
              Email is untrusted input. We treat it that way.
            </h2>
          </div>
          <ul className="mt-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
            {securityPoints.map((p) => (
              <li key={p.title} className="border-t border-white/15 pt-4">
                <h3 className="font-display font-semibold">{p.title}</h3>
                <p className="mt-2 text-sm text-code-fg/75">{p.body}</p>
              </li>
            ))}
          </ul>
          <p className="mt-10">
            <Link href="/security/" className="text-sm font-medium text-accent underline underline-offset-4">
              How we secure the platform
            </Link>
          </p>
        </Container>
      </section>

      <section id="eu" aria-labelledby="eu-title" className="py-20">
        <Container className="grid gap-10 lg:grid-cols-2">
          <SectionHeading
            id="eu-title"
            eyebrow="EU hosting"
            title="Stored in Germany. On every plan."
            intro="EU data residency is not an enterprise add on here. Mailboxes, messages and attachments are stored on Hetzner servers in Germany, for every plan including Free."
          />
          <dl className="grid gap-4 self-end text-sm sm:grid-cols-2">
            <div className="rounded-xl border border-border bg-card p-5">
              <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase">Storage and compute</dt>
              <dd className="mt-2">Hetzner Online GmbH, Germany</dd>
            </div>
            <div className="rounded-xl border border-border bg-card p-5">
              <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase">Outbound relay</dt>
              <dd className="mt-2">Brevo, France, until direct delivery is enabled</dd>
            </div>
            <div className="rounded-xl border border-border bg-card p-5">
              <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase">Backups</dt>
              <dd className="mt-2">Nightly, to Hetzner Object Storage in the EU</dd>
            </div>
            <div className="rounded-xl border border-border bg-card p-5">
              <dt className="font-mono text-xs tracking-[0.08em] text-muted uppercase">This website</dt>
              <dd className="mt-2">No cookies, no analytics, fonts served from our own server</dd>
            </div>
          </dl>
        </Container>
      </section>

      <section id="agents" aria-labelledby="agents-title" className="py-4">
        <Container>
          <div className="rounded-2xl border border-border bg-card p-6 sm:p-10">
            <SectionHeading
              id="agents-title"
              eyebrow="For agents"
              title="Give your agent access"
              intro={
                <>
                  Paste this into your coding agent. It points to{" "}
                  <a className="text-primary underline underline-offset-2" href="/llms.txt">
                    /llms.txt
                  </a>
                  , an index of the docs written for models, with a Markdown version of every page.
                </>
              }
            />
            <figure className="mt-8 overflow-hidden rounded-xl border border-border bg-code-bg text-code-fg">
              <figcaption className="flex min-h-11 items-center justify-between gap-3 border-b border-white/10 px-4 py-1">
                <span className="font-mono text-xs tracking-[0.08em] text-code-fg/70 uppercase">Prompt</span>
                <CopyButton text={prompt} label="Copy prompt" />
              </figcaption>
              <p className="p-4 font-mono text-sm leading-relaxed">{prompt}</p>
            </figure>
          </div>
        </Container>
      </section>

      <section aria-labelledby="cta-title" className="py-20">
        <Container className="text-center">
          <h2 id="cta-title" className="font-display text-3xl font-semibold tracking-tight sm:text-4xl">
            Try it on staging
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-muted">
            Self service sign up is not open yet. Ask for a staging API key and we will set you up on the Free plan.
          </p>
          <div className="mt-8 flex flex-wrap justify-center gap-3">
            <ButtonLink href={`mailto:${CONTACT_EMAIL}?subject=Staging%20API%20key`}>Request a staging key</ButtonLink>
            <ButtonLink href="/docs/" variant="secondary">
              Read the docs
            </ButtonLink>
          </div>
        </Container>
      </section>
    </>
  );
}
