# AgentMail reverse engineering pack

Target: <https://www.agentmail.to/> (marketing site), <https://docs.agentmail.to/> (docs + API), <https://console.agentmail.to/> (dashboard).
Goal: understand the product end to end and turn it into a buildable spec for **multi.ai.inbox** (this repo).
Captured: 2026-10-22. Authors: CPO + CTO hats.

## Method (REA, "Reverse Engineer Anything")

We followed the staged investigation workflow of the `reverse-engineer-anything` skill from
[morluto/rea](https://github.com/morluto/rea) (MIT, commit `746a41937d0c05fc8c53872c56f663b8b8f90b62`, skill metadata version 45):

1. Turn the request into a checklist of questions and the evidence each answer needs.
2. Start from the smallest useful inventory (sitemap, nav, `llms.txt`, OpenAPI index), then follow each feature from entry point to data and state.
3. Corroborate each conclusion with the evidence type it needs; keep negative searches bounded.
4. Keep a finding ledger: every claim is tagged **[observed]** (seen in a public page or doc), **[inferred]** (reasoned from observed facts) or **[decision]** (our own design choice for multi.ai.inbox).
5. Before finishing, revisit the checklist and mark each question answered, partial or open.

REA's browser and network capture tools (`capture_browser_scenario`, `inspect_web_network_capture`) need direct access to the target. The session's egress policy blocked `www.agentmail.to` and `docs.agentmail.to`, so runtime capture (HAR, response headers, third party scripts) was **not** performed. Evidence comes from rendered page content, screenshots, the public docs index (`llms.txt`) and the docs pages, fetched through Firecrawl. Anything that would need a HAR is marked open in the ledger below.

No private endpoint was probed, no account was created and no authenticated console page was accessed.

## Documents

| # | Document | Owner hat | Content |
|---|----------|-----------|---------|
| 01 | [PRD](01-prd.md) | CPO | Problem, ICP, positioning, goals, scope, pricing, metrics |
| 02 | [User stories](02-user-stories.md) | CPO | Personas, epics, stories with acceptance criteria |
| 03 | [Pages and UI layout](03-pages-and-ui-layout.md) | CPO | Sitemap, marketing page anatomy, console screens, wireframes |
| 04 | [Branding](04-branding.md) | CPO | Observed AgentMail brand system and a distinct brand for multi.ai.inbox |
| 05 | [Features and API spec](05-features-and-api.md) | CTO | Resource model, endpoints, events, limits, errors |
| 06 | [Architecture, infrastructure, scalability](06-architecture-infrastructure.md) | CTO | Target architecture on top of mailcow, data model, scaling path, costs |
| 07 | [Integrations](07-integrations.md) | CTO | SDKs, MCP, agent frameworks, registrars, billing, observability |
| 08 | [Security and compliance](08-security-compliance.md) | CTO | Threat model, Agent Armor equivalent, RLS, abuse prevention, GDPR |
| 09 | [Execution plan](09-execution-plan.md) | CPO + CTO | Phased roadmap, milestones, team of one, risks, decisions to take |

## Finding ledger (summary)

| Question | Status | Main evidence |
|----------|--------|---------------|
| What is the product and for whom? | answered | Home, product, enterprise pages [observed] |
| Pricing and limits | answered | `/pricing` table [observed] |
| Information architecture of the site | answered | Nav, footer, sitemap links [observed] |
| Brand system (colors, fonts, motifs) | answered | Firecrawl branding extraction + screenshots [observed] |
| API resources and endpoints | answered | `docs.agentmail.to/llms.txt` + API reference pages [observed] |
| Event model (webhooks, websockets) | answered | Webhook event list, AsyncAPI mention [observed] |
| Frontend stack | partial | `/_next/image`, `next-size-adjust`, `dpl_` ids point to Next.js on Vercel [inferred] |
| Third party scripts and analytics | open | Needs HAR capture (blocked) |
| Console (dashboard) screens | partial | Inferred from docs and pricing copy; no authenticated access |
| Email provider | answered | DNS records in custom domain docs point to Amazon SES (us-east-1, eu-central-1) [observed] |
| Backend services stack | open | Not public; our architecture is a [decision], not a copy |
| Agent Armor model internals | open | Only public description available |

## Legal and ethical boundary

We study a public product to learn the shape of the problem. We do **not** copy AgentMail's name, logo, illustrations, copywriting or proprietary code. The branding document describes their system only as reference and defines a separate identity for multi.ai.inbox. API compatibility (same resource names, same verbs) is a deliberate product choice to lower switching cost; it is not a copy of their implementation.
