# 03 · Pages, information architecture and UI layout

## 1. Observed properties of the reference

| Host | Role | Evidence |
|------|------|----------|
| `www.agentmail.to` | Marketing site, blog, legal | observed |
| `docs.agentmail.to` | Docs, API reference, `llms.txt`, OpenAPI 3.1, AsyncAPI 2.6, hosted docs MCP at `/_mcp/server` | observed |
| `console.agentmail.to` | Dashboard: sign up, sign in, keys, inboxes, billing | observed |
| `status.agentmail.to` | Status page | observed |
| `app.vanta.com/agentmail.to/trust/...` | Trust center | observed |
| `agentid.com` | Sister product for agent identity | observed |

### Marketing sitemap [observed]

```
/                      Home
/product               Product deep dive
/features              Feature list
/use-cases             Use cases by function and industry
/enterprise            Enterprise + contact form + Cal.com booking
/pricing               Calculator + plan cards + add ons + comparison table (noindex)
/security              SOC 2, Agent Armor, lists, permissions, compliance FAQ
/about                 Company
/careers               Jobs
/blog, /blog/<slug>    Customer stories, launches
/legal/privacy | terms | soc2 | subprocessors | copyright
```

Top nav: `PRODUCT ▾` · `USE CASES` · `ENTERPRISE` · `PRICING` · `RESOURCES ▾` · `ABOUT`, right side `Start for free →` (light button) and `See how it works` (dark button).
Footer: logo, NAVIGATION (Product, Enterprise, Use cases, Features, Security, Pricing, About), SOCIALS (Discord, X, GitHub, LinkedIn, Y Combinator), support email, newsletter field, secondary row (Blog, Careers, AgentID, Docs, Console), legal row with cookie settings.

### Home page anatomy [observed from screenshots, 1440 px]

| # | Section | Layout | Content pattern |
|---|---------|--------|-----------------|
| 1 | Hero | 2 columns: H1 left ("Email Inboxes for AI agents"), tagline + 2 CTAs right; below, a 3D stack of envelopes (orange, beige, blue) on a wireframe globe with coordinate labels | Product as a physical object |
| 2 | Logo bar | Rounded dark card, mono label "AI COMPANIES BUILDING ON AGENTMAIL", 8 logos | Social proof |
| 3 | Product | Left: numbered accordion 01 to 06 (Two way conversations, Send at scale, Process incoming, Coordinate schedules, Use services independently, Work with other agents). Right: mockup image swapped per item | Use case tour |
| 4 | Features | Left: 3 envelope tabs (01 Programmatic Inboxes, 02 Events and context, 03 Control and scale). Right: panel with 4 mono headed items and an illustration | Capability grid |
| 5 | Scale stats | 3 staggered cards: 1B+ requests, 1B+ emails, 10M+ inboxes, dotted map art | Traction |
| 6 | Testimonials | Carousel: customer logo, avatar, quote, "Read Story" | Proof |
| 7 | Code CTA | Tabs Python, TypeScript, cURL, CLI, MCP, Skills; install command + 6 line snippet | Developer hook |
| 8 | Security and control | 3 cards: Security (SOC 2), Permissions, Pods; 2 CTAs | Objection handling |
| 9 | Agent prompt CTA | Light card with a terminal "prompt" box the user copies into their agent, plus Start for free / Sign in | Agent led onboarding |
| 10 | Footer | as above | |

Notable pattern: section 9 sells to the **agent**, not the human. The copy is a prompt that tells the agent to read `agent-signup` and `llms.txt`. We replicate this mechanic [decision].

### Pricing page anatomy [observed]
Toggle Inboxes / Emails / Domains and Monthly / Yearly; slider (10, 25, 50, 100, 150 inboxes) that recommends a plan; 4 plan cards; add ons grid; comparison table grouped Core, Advanced, Deliverability, Security, Support.

### Console [partially observed]
* Auth by Clerk (`img.clerk.com` assets), Google social plus email and password, light and dark mode toggle, ASCII art logo [observed].
* Sign up hint: "Use the email you want as your login, rather than an @agentmail.to inbox address" [observed].
* Inferred screens from docs and pricing copy: Overview, Inboxes, Threads viewer, Drafts, Domains, Webhooks, API Keys, Pods, Metrics, Billing (add ons, invoices), Members, Settings, Labs [inferred].

## 2. multi.ai.inbox page map [decision]

Next.js App Router, following the founder's directory convention.

```
src/app/
├── (public)/
│   ├── page.tsx                 Home
│   ├── product/page.tsx
│   ├── use-cases/page.tsx
│   ├── pricing/page.tsx
│   ├── security/page.tsx
│   ├── eu/page.tsx              EU residency story (our wedge)
│   ├── self-host/page.tsx       Open core + mailcow
│   ├── blog/[slug]/page.tsx     MDX
│   ├── legal/[doc]/page.tsx
│   ├── llms.txt/route.ts        Docs index for agents
│   └── (auth)/sign-in, sign-up
├── (auth)/console/
│   ├── page.tsx                 Overview: usage, quickstart, recent events
│   ├── inboxes/page.tsx         Table + create dialog
│   ├── inboxes/[id]/page.tsx    Thread list + reader + compose
│   ├── drafts/page.tsx          Approval queue
│   ├── review/page.tsx          Security review queue (Armor)
│   ├── domains/page.tsx         Domains + registrar connect
│   ├── domains/[id]/page.tsx    DNS records, verification status
│   ├── webhooks/page.tsx        Endpoints + delivery log + replay
│   ├── keys/page.tsx            API keys + scopes
│   ├── pods/page.tsx            Tenants
│   ├── metrics/page.tsx
│   ├── billing/page.tsx
│   └── settings/page.tsx        Org, members, danger zone
├── api/                         BFF for the console only (public API lives in Go service)
└── middleware.ts                Supabase session guard for (auth)
```

Docs: Mintlify free tier or Fumadocs inside the same Next app [decision: Fumadocs, zero cost, MDX, generates `.md` per page].

## 3. Console wireframes [decision]

Mobile first: the sidebar collapses into a bottom sheet under 768 px; tables become cards.

### Overview
```
┌──────────────┬──────────────────────────────────────────────────────────┐
│ ▣ multi.ai   │  Overview                                   [EU · fsn1]  │
│              │ ┌──────────┐┌──────────┐┌──────────┐┌──────────┐         │
│ Overview     │ │Inboxes   ││Sent / mo ││Received  ││Storage   │         │
│ Inboxes      │ │ 3 / 3    ││ 120/3000 ││ 418      ││ 0.2/3 GB │         │
│ Drafts   (2) │ └──────────┘└──────────┘└──────────┘└──────────┘         │
│ Review   (1) │  Quickstart  [Python][TS][cURL][MCP][Prompt]              │
│ Domains      │ ┌──────────────────────────────────────────────────────┐ │
│ Webhooks     │ │ pip install multi-ai-inbox                    [copy] │ │
│ API keys     │ │ client.inboxes.create()                              │ │
│ Pods         │ └──────────────────────────────────────────────────────┘ │
│ Metrics      │  Recent events                                           │
│ Billing      │  12:01 message.received  sales@acme.ai  "Re: quote"      │
│ Settings     │  11:58 message.sent      ops@acme.ai    "Invoice 42"      │
└──────────────┴──────────────────────────────────────────────────────────┘
```

### Inbox reader (3 pane on desktop, stacked on mobile)
```
┌ Inboxes ───────┬ Threads (label: all ▾) ─┬ Thread ──────────────────────┐
│ sales@acme.ai  │ ● Vendor X  Re: quote   │ Re: quote          [labels]  │
│ ops@acme.ai    │   Bank     Statement    │ ─ Vendor X  Thu 09:12        │
│ + New inbox    │   Lisa     Meeting?     │   extracted_text …           │
│                │                         │   📎 quote.pdf (text ready)  │
│                │                         │ ─ agent     Thu 09:13        │
│                │                         │ [Reply] [Forward] [Raw .eml] │
└────────────────┴─────────────────────────┴──────────────────────────────┘
```

### Domain setup
```
acme.ai                                          Status: ◐ verifying (DKIM ✓ SPF ✗ MX ✗)
[ Connect Namecheap ] [ Connect Infomaniak ] [ Download zone file ]
TYPE   HOST                         VALUE                                   STATE
MX     acme.ai                      10 mx.multi-ai-inbox.eu                  ✗
TXT    acme.ai                      v=spf1 include:spf.multi-ai-inbox.eu ~all ✗
CNAME  mai1._domainkey.acme.ai      mai1.dkim.multi-ai-inbox.eu              ✓
TXT    _dmarc.acme.ai               v=DMARC1; p=none; rua=mailto:…           ✓
⚠ Existing MX found (Google Workspace). Use a subdomain like agents.acme.ai?
```

## 4. Component states (every component)

Per founder rules each data component ships **loading** (skeleton), **error** (inline message + retry + toast) and **empty** (illustration + primary action) states. Examples:

| Component | Empty state | Error state |
|-----------|-------------|-------------|
| Inbox table | "No inboxes yet" + Create inbox + code snippet | "Could not load inboxes" + Retry |
| Thread list | "Waiting for the first email" + copy address button + "send a test email" | toast + retry |
| Webhook log | "No deliveries yet" + "Send test event" | per row failure reason + Replay |
| Domains | "Use the shared domain or add your own" | DNS lookup error with record diff |
