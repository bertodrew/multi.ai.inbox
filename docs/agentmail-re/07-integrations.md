# 07 · Integrations

## 1. Reference integration surface [observed]

| Category | AgentMail offers |
|----------|------------------|
| SDKs | Python (`pip install agentmail`), TypeScript, plus cURL; `agentmail-toolkit` packages for AI SDK and OpenAI Agents SDK |
| Agent clients | Hosted MCP server, MCP Events (push to ChatGPT), Skills for Claude Code / Cursor / OpenClaw, CLI |
| Frameworks | Google ADK, LangChain, Vercel AI SDK, OpenAI Agents SDK, Claude Agent SDK, LiveKit Agents, Sim.ai |
| Platforms | Replit, Vercel Marketplace (provisioning), Clay (email as a column), Grok |
| Payments for agents | x402 (HTTP native payments), MPP (machine payments via Stripe) |
| Identity | AgentID: sign in to third party apps, P 256 public key auth, app accounts |
| Email clients | IMAP and SMTP access |
| Google Workspace | Route unknown addresses of a Workspace domain to AgentMail |
| DNS guides | Cloudflare, GoDaddy, Route 53, Namecheap |
| Docs for agents | `llms.txt`, `.md` per page, hosted docs MCP at `docs.agentmail.to/_mcp/server` |
| Trust / ops | Vanta trust center, status page, Discord, Cal.com for sales calls |
| Auth for console | Clerk with Google social login |
| Webhook infra | Svix style signing and polling endpoints [inferred from "read its events with the Svix SDK"] |

## 2. multi.ai.inbox integration plan [decision]

Priority is by reach per hour of work for a solo founder.

| Phase | Integration | Effort | Notes |
|-------|-------------|--------|-------|
| P0 | REST + OpenAPI 3.1 | M | Single source of truth in the Go repo; generate Go server stubs (oapi-codegen) and Zod schemas (openapi-zod-client) for the web BFF |
| P0 | Webhooks signed Svix compatible | S | Lets customers reuse Svix verification libraries |
| P1 | TypeScript SDK | S | Generated (openapi-typescript + thin client), npm `multi-ai-inbox` |
| P1 | Python SDK | S | Generated (openapi-python-client), PyPI |
| P1 | Hosted MCP server | S | Extend the fork's existing MCP server (currently `list_domains`, `get/set_domain_settings`) with inbox, send, threads, messages, labels tools |
| P1 | Claude Code skill + `llms.txt` | S | Ship a `SKILL.md` in a public repo (MIT) and a prompt CTA on the site |
| P1 | CLI | S | Go single binary (`mai inbox create`, `mai send`, `mai tail`) |
| P1 | Registrar connectors | done/S | Namecheap (API key) and Infomaniak (OAuth v2) already designed in this fork; add Cloudflare (API token) next because most devs use it |
| P1 | Stripe Billing | M | Checkout, Customer Portal, metered add ons; webhooks to update `subscriptions` |
| P2 | Vercel AI SDK + OpenAI Agents SDK toolkits | S | Thin wrappers over the TS / Python SDK |
| P2 | LangChain tool | S | Python toolkit |
| P2 | WebSocket client in SDKs | S | `client.events.stream()` |
| P2 | IMAP / SMTP | M | Dovecot with a custom passdb (API key as password) and SMTP submission mapped to `send` |
| P2 | Google Workspace / Microsoft 365 routing guide | S | Docs only |
| P3 | Vercel Marketplace, Replit template | M | Distribution |
| P3 | Calendar (iMIP), CalDAV export | L | |
| P3 | Agent identity / sign in for apps | L | Watch market first |
| P3 | x402 / Stripe agent payments | M | Only if customers ask |

## 3. Internal tooling integrations

| Need | Choice | Cost |
|------|--------|------|
| Console auth | Supabase Auth (Google, email, MFA TOTP) instead of Clerk, to keep users and RLS in one place | free |
| Product analytics | PostHog EU cloud | free tier |
| Errors | Sentry | free tier |
| Metrics and traces | Grafana Cloud | free tier |
| Status page | Upptime on GitHub Pages | free |
| Support | Discord community + email; Plain or Crisp later | free |
| Sales calls | Cal.com | free |
| Trust center | Static `/security` page first; Vanta only after revenue (SOC 2 is expensive) | free |
| Email for our own transactional mail | our own platform (dogfooding) | free |

## 4. Webhook contract (Svix compatible)

The reference signs with Svix headers `svix-id`, `svix-timestamp`, `svix-signature` [observed]. We emit both header families with the same value. Headers: `webhook-id`, `webhook-timestamp`, `webhook-signature` (`v1,<base64 HMAC SHA256 of "{id}.{timestamp}.{body}">`). Secret format `whsec_<base64>`. Reject timestamps older than 5 minutes. Retry schedule: immediately, 5 s, 5 min, 30 min, 2 h, 5 h, 10 h, 10 h (about 28 h total), then disable after 5 days of failures with an email to the owner.
