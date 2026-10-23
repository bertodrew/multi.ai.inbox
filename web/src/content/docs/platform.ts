import { API_URL, MCP_URL } from "@/lib/site";
import { apiReference } from "@/generated/openapi";
import { mcpClaudeCode, mcpCodex, mcpCursor, verifyPython, verifyTs } from "../snippets";
import type { DocPage } from "../types";

const eventDescriptions: Record<string, string> = {
  "message.received": "A message arrived in an inbox. With screen before notify (the default) it fires only after a clean verdict, or after a release.",
  "message.received.spam": "A message arrived and the spam filter flagged it (it carries the `spam` label).",
  "message.sent": "An outbound message was accepted by the next hop.",
  "message.failed": "An outbound message failed after all retries.",
  "message.bounced": "A delivery status notification reported that an outbound message could not be delivered (it gets the `bounced` label).",
  "message.complained": "A recipient reported an outbound message as spam through a feedback loop (it gets the `complained` label).",
  "message.security.completed": "Gatekeeper screening finished; `extracted_text` and the verdict are set.",
  "message.security.review": "Gatekeeper screening finished with verdict `review` (or could not run). With screen before notify the message is held and the payload carries no body.",
  "domain.verified": "All DNS records of a custom domain were found.",
  "draft.created": "A draft was saved in an inbox. The payload carries the draft without attachment content.",
  "inbox.sending.paused": "The bounce breaker paused sending from an inbox: hard bounce rate above 5% over at least 20 sends in 24 hours, spam complaints above 0.1% over at least 1000 sends, or 3 complaints in 24 hours below that volume.",
  "inbox.sending.resumed": "Support resumed sending from an inbox.",
  "organization.sending.paused": "The bounce breaker paused sending for the whole organization.",
  "organization.sending.resumed": "Support resumed sending for the organization.",
  "security.av_unavailable": "The virus scanner was unavailable and content was accepted with the `av-unscanned` label. Sent at most once per 10 minutes.",
};

export const customDomains: DocPage = {
  slug: "custom-domains",
  title: "Custom domains",
  description: "Send and receive from your own domain: DNS records, verification and limits.",
  blocks: [
    {
      type: "p",
      text: "Use your own domain (or a subdomain such as `agents.your-domain.eu`) so recipients see your brand. A subdomain is recommended: it keeps agent mail separate from your team's mail.",
    },
    { type: "h2", text: "1. Add the domain" },
    {
      type: "code",
      sample: {
        label: "cURL",
        lang: "bash",
        code: `curl -s -X POST ${API_URL}/v0/domains \\
  -H "Authorization: Bearer $MAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"domain": "agents.your-domain.eu"}'`,
      },
    },
    {
      type: "p",
      text: "The response contains the DNS records to publish. A fresh DKIM key is generated for each domain. Copy the values exactly as returned; the table shows their shape.",
    },
    {
      type: "table",
      head: ["Type", "Name", "Value", "Purpose"],
      rows: [
        ["MX", "`agents.your-domain.eu`", "`10 <platform mail host>.`", "Routes inbound mail to the platform."],
        ["TXT", "`agents.your-domain.eu`", "`v=spf1 mx include:<relay> -all`", "SPF: authorizes the platform to send."],
        ["TXT", "`<selector>._domainkey.agents.your-domain.eu`", "`v=DKIM1; k=rsa; p=<public key>`", "DKIM: public key for signatures."],
        ["TXT", "`_dmarc.agents.your-domain.eu`", "`v=DMARC1; p=quarantine; rua=mailto:dmarc@agents.your-domain.eu`", "DMARC policy."],
      ],
    },
    {
      type: "note",
      tone: "warn",
      text: "Publishing an MX record moves inbound mail for that name to the platform. Do not add it to a domain whose mail you still receive elsewhere; use a subdomain instead.",
    },
    { type: "h2", text: "2. Verify" },
    {
      type: "p",
      text: "Call `POST /v0/domains/{domain}/verify` to check DNS now. The scheduler also checks every 5 minutes for 7 days after you add the domain. Each record reports `verified` or `missing`; when all four are found the domain becomes `verified` and the `domain.verified` event fires.",
    },
    { type: "h2", text: "3. Create inboxes on it" },
    {
      type: "p",
      text: "Pass `domain` when you [create an inbox](/docs/inboxes). Until the domain is verified the call returns `403 domain_not_verified`.",
    },
    { type: "h2", text: "Limits and deletion" },
    {
      type: "list",
      items: [
        "The number of custom domains depends on the plan: 1 on Free, 5 on Builder, 20 on Growth, 50 on Team.",
        "A domain can be deleted only when it has no inboxes left; otherwise the API returns `409 cannot_delete`.",
        "`GET /v0/domains` lists your domains and the shared platform domain.",
      ],
    },
    { type: "h2", text: "Endpoints" },
    {
      type: "endpoints",
      keys: ["POST /v0/domains", "GET /v0/domains", "GET /v0/domains/{domain}", "DELETE /v0/domains/{domain}", "POST /v0/domains/{domain}/verify"],
    },
    { type: "schemas", names: ["Domain"] },
  ],
};

export const webhooks: DocPage = {
  slug: "webhooks",
  title: "Webhooks",
  description: "Signed event delivery: event types, payload, retries and signature verification.",
  blocks: [
    {
      type: "p",
      text: "Webhooks push events to your HTTPS endpoint as they happen. Deliveries follow the [Standard Webhooks](https://www.standardwebhooks.com/) scheme, and the same headers are also sent with the `svix-` prefix, so existing Standard Webhooks or Svix verification libraries work unchanged.",
    },
    { type: "h2", text: "Create a webhook" },
    {
      type: "code",
      sample: {
        label: "cURL",
        lang: "bash",
        code: `curl -s -X POST ${API_URL}/v0/webhooks \\
  -H "Authorization: Bearer $MAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"url": "https://agent.your-domain.eu/hooks/mail", "event_types": ["message.received", "message.security.review"]}'`,
      },
    },
    {
      type: "p",
      text: "The response contains the signing `secret` (`whsec_...`). It is returned only once. Optionally restrict a webhook to up to 10 inboxes with `inbox_ids`.",
    },
    { type: "h2", text: "Event types" },
    {
      type: "table",
      head: ["Event", "When"],
      rows: apiReference.eventTypes.map((e) => [`\`${e}\``, eventDescriptions[e] ?? ""]),
    },
    { type: "h2", text: "Payload" },
    {
      type: "p",
      text: "Message events carry the message and its thread. `domain.verified` carries the domain. Sending pause events carry the inbox or organization, the reason and the counts that tripped the breaker.",
    },
    {
      type: "code",
      sample: {
        label: "POST body",
        lang: "json",
        code: `{
  "type": "event",
  "event_type": "message.received",
  "event_id": "5f0c7a2e-6b1d-4a57-9d1e-2f9a1c3b8e44",
  "created_at": "2026-10-22T09:41:07Z",
  "data": {
    "message": { "id": "...", "inbox_id": "billing@agents.your-domain.eu", "subject": "Invoice 2026-118", "labels": ["received", "unread"] },
    "thread": { "thread_id": "...", "message_count": 1 }
  }
}`,
      },
    },
    {
      type: "p",
      text: "With screen before notify, on by default for every organization, `message.received` fires only after the gatekeeper verdict is `clear`: first `message.security.completed`, then `message.received`. A message held for review fires only `message.security.review`, with `text`, `html`, `extracted_text` and previews left out, and fires `message.received` once a key with `message_release` releases it. Deliveries leave in event order on the first attempt; retries can arrive later than newer events. If an operator turns the setting off, `message.received` fires at ingest while the gatekeeper may still be running.",
    },
    { type: "h2", text: "Signature" },
    {
      type: "table",
      head: ["Header", "Value"],
      rows: [
        ["`webhook-id`", "`msg_<event id>`, stable across retries. Use it to deduplicate."],
        ["`webhook-timestamp`", "Unix seconds when the attempt was signed."],
        ["`webhook-signature`", "`v1,<base64 HMAC-SHA256>` (a space separated list)."],
        ["`svix-id`, `svix-timestamp`, `svix-signature`", "The same values, for Svix libraries."],
      ],
    },
    {
      type: "p",
      text: "The signature is an HMAC-SHA256 over `{webhook-id}.{webhook-timestamp}.{raw body}`, keyed with the base64 decoded part of the secret after `whsec_`. Reject timestamps more than 5 minutes away from your clock, and compare in constant time. Verify against the raw body bytes: parsing and re-serializing JSON changes them.",
    },
    { type: "tabs", samples: [verifyTs, verifyPython] },
    { type: "h2", text: "Delivery and retries" },
    {
      type: "list",
      items: [
        "Any `2xx` status within 15 seconds counts as delivered. Redirects are not followed.",
        "Failed deliveries are retried after 5 seconds, 5 minutes, 30 minutes, 2 hours, 5 hours, 10 hours and 10 hours, then marked failed.",
        "Events can arrive more than once and out of order. Deduplicate with `webhook-id` and read the current state from the API when order matters.",
        "Webhook URLs must be HTTPS and resolve to public addresses. Private, loopback and link local addresses are refused.",
      ],
    },
    { type: "h2", text: "Event stream" },
    {
      type: "p",
      text: "For live updates without a public endpoint, open `GET /v0/events/stream` with a key holding `message_read`. It is a Server Sent Events stream: each event has an `id`, the event type as `event` and the webhook body above as `data`, with the same redaction for held mail. Narrow it with `?types=message.received,message.sent` and `?inbox=<address>`. A client that reconnects with `Last-Event-ID` (EventSource does this for you) continues after that event; without it the stream starts with new events.",
    },
    {
      type: "code",
      sample: {
        label: "cURL",
        lang: "bash",
        code: `curl -N ${API_URL}/v0/events/stream?types=message.received \\
  -H "Authorization: Bearer $MAI_API_KEY"`,
      },
    },
    {
      type: "list",
      items: [
        "Up to 5 open streams per API key; more return `429 too_many_streams`.",
        "An idle stream gets a `: ping` comment every 15 seconds, so proxies keep it open.",
        "A revoked or expired key loses its open streams within a minute.",
        "Delivery is at least once: deduplicate with `event_id`. Use webhooks when events must reach you while no client is connected.",
      ],
    },
    { type: "h2", text: "Endpoints" },
    { type: "endpoints", keys: ["POST /v0/webhooks", "GET /v0/webhooks", "DELETE /v0/webhooks/{webhook}", "GET /v0/events/stream"] },
    { type: "schemas", names: ["Webhook"] },
  ],
};

export const mcp: DocPage = {
  slug: "mcp",
  title: "MCP",
  description: "Use inboxes as MCP tools from Claude Code, Cursor, Codex and other MCP clients.",
  blocks: [
    {
      type: "p",
      text: `The MCP server runs at \`${MCP_URL}\` over streamable HTTP. It uses the same API key as REST, in \`X-Api-Key\` or \`Authorization: Bearer\`, and the same rules: plan limits, idempotency, organization isolation and [scoped permissions](/docs/authentication#scoped-keys). Tool calls are written to the audit log.`,
    },
    { type: "h2", text: "Claude Code" },
    { type: "code", sample: mcpClaudeCode },
    { type: "h2", text: "Cursor" },
    { type: "p", text: "Add the server to `.cursor/mcp.json` in your project, or to the global MCP settings:" },
    { type: "code", sample: mcpCursor },
    { type: "h2", text: "Codex" },
    { type: "code", sample: mcpCodex },
    {
      type: "p",
      text: "Other clients: any MCP client that supports streamable HTTP with a custom header or a bearer token can connect with the same URL. Give agents a [scoped key](/docs/authentication#scoped-keys) with only the permissions they need.",
    },
    { type: "h2", text: "Tools" },
    {
      type: "table",
      head: ["Tool", "What it does"],
      rows: [
        ["`create_inbox`", "Create an email inbox for an agent. Optional `username`, `domain`, `display_name`, `client_id`."],
        ["`list_inboxes`", "List inboxes, newest first."],
        ["`delete_inbox`", "Delete an inbox with all its mail. Irreversible; the address stays reserved for 90 days."],
        ["`list_threads`", "List conversation threads of an inbox, or of all inboxes."],
        ["`get_thread`", "Get a thread with the messages the key may see, oldest first."],
        ["`list_messages`", "List message metadata of an inbox (no bodies)."],
        ["`get_message`", "Get one message with body, `extracted_text`, authentication results and gatekeeper verdict."],
        ["`send_message`", "Send a new email from an inbox. Starts a new thread. Accepts `idempotency_key`."],
        ["`reply_to_message`", "Reply in the same thread; threading headers are set for you. Accepts `idempotency_key`."],
        ["`update_message_labels`", "Add or remove custom labels to track workflow state."],
        ["`create_draft`", "Save an email as a draft for review or later sending; `in_reply_to` keeps the thread, `send_at` schedules it."],
        ["`list_drafts`", "List drafts of an inbox, newest first."],
        ["`send_draft`", "Send a draft now; it is deleted once queued. Accepts `idempotency_key`."],
        ["`get_attachment`", "Download an attachment: text files as text, others as base64 (max 5 MB)."],
        ["`list_domains`", "List domains usable for inboxes, with DNS records and verification status."],
        ["`release_message`", "Release a message held for security review. Needs `message_release`; meant for a human reviewer, not the agent."],
      ],
    },
    {
      type: "note",
      tone: "info",
      text: "Tool descriptions are short on purpose, to save the agent's context. `get_message` reminds the model to check the gatekeeper verdict before acting on instructions in the email. The read tools hide messages held by screening; they accept `include_review: true` only from keys holding `message_review`.",
    },
  ],
};

const errorMeaning: Record<string, [string, string]> = {
  missing_authorization: ["401", "No API key in the request."],
  unknown_api_key: ["401", "The API key is not valid."],
  validation_error: ["400", "A field is missing or malformed. The message names it."],
  not_found: ["404", "The resource does not exist in your organization."],
  already_exists: ["409", "The inbox address or domain is taken."],
  domain_not_verified: ["403", "The domain is not verified, or does not belong to your organization."],
  conflict: ["409", "Idempotency-Key reused with a different request, or the first request is still running."],
  cannot_delete: ["409", "The domain still has inboxes."],
  limit_exceeded: ["403 or 429", "A plan limit: inboxes or domains (403), daily or monthly sends (429)."],
  rate_limit_exceeded: ["429", "Too many requests. Wait for the `Retry-After` header."],
  rate_limited: ["429", "The plan's 5 minute send burst limit is reached. Retry after the `Retry-After` seconds."],
  address_quarantined: ["409", "The address belonged to an inbox deleted less than 90 days ago; it is reserved until the date in the message."],
  sending_paused: ["403", "Sending from the inbox or organization is paused because too many recent sends hard bounced. Contact support."],
  missing_permission: ["403", "The API key is scoped and lacks the permission this endpoint needs."],
  permission_escalation: ["403", "A key tried to grant or revoke permissions it does not hold."],
  api_key_expired: ["401", "The API key passed its expiry date."],
  too_many_streams: ["429", "The API key already has 5 open event streams. Close one first."],
  not_held: ["409", "Release of a message that is not held for review (still pending screening, clean or already released)."],
  payload_too_large: ["413", "Attachments or request body too large."],
  internal_error: ["500", "Unexpected error on our side. Safe to retry with the same Idempotency-Key."],
};

export const errors: DocPage = {
  slug: "errors",
  title: "Errors",
  description: "The error envelope and every error code the API returns.",
  blocks: [
    {
      type: "p",
      text: "Errors use standard HTTP status codes and one JSON envelope. Branch on `code`, which is stable; `message` is for humans and may change.",
    },
    {
      type: "code",
      sample: {
        label: "Error envelope",
        lang: "json",
        code: `{
  "name": "Forbidden",
  "code": "limit_exceeded",
  "message": "the free plan allows 3 inboxes"
}`,
      },
    },
    { type: "h2", text: "Codes" },
    {
      type: "table",
      head: ["Code", "Status", "Meaning"],
      rows: apiReference.errorCodes.map((c) => {
        const [status, meaning] = errorMeaning[c] ?? ["", ""];
        return [`\`${c}\``, status, meaning];
      }),
    },
    { type: "h2", text: "Retrying" },
    {
      type: "list",
      items: [
        "Retry `429` and `5xx` with backoff. Honor `Retry-After` when present.",
        "Do not retry `4xx` other than `429` without changing the request.",
        "Always send an `Idempotency-Key` on send and reply, so retries cannot duplicate mail.",
      ],
    },
  ],
};

export const limits: DocPage = {
  slug: "limits",
  title: "Limits",
  description: "Plan limits, rate limits and size limits enforced by the API.",
  blocks: [
    { type: "h2", text: "Plan limits" },
    {
      type: "table",
      head: ["Plan", "Inboxes", "Custom domains", "Sends per day", "Sends per month", "Footer"],
      rows: [
        ["Free", "3", "1", "100", "3,000", "Yes"],
        ["Builder", "10", "5", "1,000", "10,000", "No"],
        ["Growth", "50", "20", "2,500", "40,000", "No"],
        ["Team", "150", "50", "15,000", "150,000", "No"],
        ["Enterprise", "Custom", "Custom", "Custom", "Custom", "No"],
      ],
    },
    {
      type: "p",
      text: "Inbox and domain limits return `403 limit_exceeded`; send limits return `429 limit_exceeded`. Receiving mail is not counted against send limits. See [Pricing](/pricing).",
    },
    { type: "h2", text: "Request limits" },
    {
      type: "table",
      head: ["Limit", "Value"],
      rows: [
        ["API requests", "Per organization token bucket, 20 requests per second with bursts up to 40 by default. Over the limit: `429 rate_limit_exceeded` with `Retry-After`."],
        ["Send burst", "10% of the daily send limit per 5 minutes: Free 10, Builder 100, Team 1,500. Over the limit: `429 rate_limited` with `Retry-After`."],
        ["Failed authentications", "Throttled per client address."],
        ["JSON request body", "8 MB"],
        ["Attachments per sent message", "6 MB in total"],
        ["Recipients per sent message", "50 across `to`, `cc` and `bcc`"],
        ["Inbound message size", "25 MB"],
        ["Page size", "`limit` from 1 to 100, default 20"],
        ["Idempotency-Key", "Up to 256 characters, remembered for 24 hours"],
        ["Webhook `inbox_ids`", "Up to 10 per webhook"],
        ["Open event streams", "5 per API key"],
        ["Draft `send_at`", "In the future, at most 365 days ahead"],
        ["MCP `get_attachment`", "5 MB; use REST for larger files"],
        ["Inbox `display_name`", "200 characters"],
      ],
    },
    {
      type: "note",
      tone: "info",
      text: "These values reflect the current release and can change while the service is in staging. The API always reports the limit it applied in the error message.",
    },
  ],
};

export const apiReferencePage: DocPage = {
  slug: "api-reference",
  title: "API reference",
  description: "Every REST endpoint and object, generated from the OpenAPI file.",
  blocks: [
    {
      type: "p",
      text: `Generated from \`openapi/openapi.yaml\` (version ${apiReference.version}) at build time. Base URL: \`${API_URL}\`. The API also serves the file itself at \`${API_URL}/openapi.yaml\`.`,
    },
    { type: "h2", text: "Endpoints" },
    { type: "endpoints", keys: apiReference.endpoints.map((e) => `${e.method} ${e.path}`) },
    { type: "h2", text: "Objects" },
    { type: "schemas", names: apiReference.schemas.map((s) => s.name) },
  ],
};
