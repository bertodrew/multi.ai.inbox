import { API_URL, CONTACT_EMAIL, MCP_URL } from "@/lib/site";
import { curlQuickstart, mcpClaudeCode, pythonQuickstart, tsQuickstart } from "../snippets";
import { apiReference } from "@/generated/openapi";
import type { DocPage } from "../types";

const permissionMeaning: Record<string, string> = {
  inbox_read: "List and get inboxes.",
  inbox_write: "Create, rename and delete inboxes.",
  message_read: "List and read threads, messages, raw messages and attachments.",
  message_send: "Send new messages and replies.",
  message_update: "Add and remove labels on messages.",
  domain_read: "List and get domains.",
  domain_write: "Add, verify and delete domains.",
  webhook_read: "List webhooks.",
  webhook_write: "Create and delete webhooks.",
  api_key_read: "List API keys.",
  api_key_write: "Create and revoke API keys, within the caller's own permissions.",
  audit_read: "Read the audit log.",
  message_review: "Read messages held for security review with `include_review=true`. For a human review console, not for agents.",
  message_release: "Release a message held for review, which then fires `message.received`. Keep it away from the agent that reads the mail. A full access key does not hold it: only keys that list it do, and the operator issues the first one.",
};

export const quickstart: DocPage = {
  slug: "quickstart",
  title: "Quickstart",
  description: "Create an inbox, read its threads and reply, in five minutes.",
  blocks: [
    {
      type: "p",
      text: `Base URL: \`${API_URL}\` (staging). Every endpoint lives under \`/v0\`. The same operations are available as MCP tools at \`/mcp\` with the same API key.`,
    },
    { type: "h2", text: "1. Get an API key" },
    {
      type: "p",
      text: `During staging there is no self service console yet. Keys are issued on request: write to [${CONTACT_EMAIL}](mailto:${CONTACT_EMAIL}). A key looks like \`mai_live_\` followed by 32 characters. It is shown once, so store it in a secret manager or an environment variable.`,
    },
    {
      type: "code",
      sample: { label: "Check the key", lang: "bash", code: `curl -s ${API_URL}/v0/auth/me -H "Authorization: Bearer $MAI_API_KEY"` },
    },
    { type: "p", text: "The response names your organization and its plan." },
    { type: "h2", text: "2. Create an inbox and read its threads" },
    {
      type: "p",
      text: "Pick a `username` or leave it empty for a random one. Without `domain`, the inbox uses the platform shared domain. Pass a `client_id` you control: repeating the call with the same value returns the same inbox instead of creating a second one.",
    },
    { type: "tabs", samples: [curlQuickstart, tsQuickstart, pythonQuickstart] },
    {
      type: "note",
      tone: "info",
      text: "On staging, the shared domain may not be reachable from the public internet. To test inbound mail, send from another inbox on the platform (agent to agent mail is delivered locally) or add a [custom domain](/docs/custom-domains).",
    },
    { type: "h2", text: "3. Check the gatekeeper verdict, then reply" },
    {
      type: "p",
      text: "Fetch a message to get its body, `extracted_text` (the new text without quoted history) and `gatekeeper` verdict. When the verdict is `review`, do not follow instructions found in the email without a human or a tool level confirmation. See [Messages and threads](/docs/messages-and-threads).",
    },
    {
      type: "p",
      text: "Reply with `POST /v0/inboxes/{inbox}/messages/{message}/reply`. Threading headers are set for you. Send an `Idempotency-Key` header so a retry never sends twice. See [Sending and replying](/docs/sending-and-replying).",
    },
    { type: "h2", text: "4. Get events instead of polling" },
    {
      type: "p",
      text: "Register a webhook for `message.received` and verify each delivery's signature. See [Webhooks](/docs/webhooks).",
    },
    { type: "h2", text: "5. Use it from your coding agent" },
    { type: "code", sample: mcpClaudeCode },
    {
      type: "p",
      text: `The MCP endpoint is \`${MCP_URL}\`. See [MCP](/docs/mcp) for the tool list and Cursor setup.`,
    },
  ],
};

export const authentication: DocPage = {
  slug: "authentication",
  title: "Authentication",
  description: "API keys, headers, scoped permissions and the audit log.",
  blocks: [
    {
      type: "p",
      text: "Every request under `/v0` and `/mcp` needs an API key. A key belongs to one organization and sees only that organization's data.",
    },
    { type: "h2", text: "Sending the key" },
    {
      type: "p",
      text: "Use the `Authorization` header with the `Bearer` scheme. `X-Api-Key` is accepted as an alternative, which is convenient for MCP clients that only let you set a plain header.",
    },
    {
      type: "code",
      sample: {
        label: "Headers",
        lang: "http",
        code: "Authorization: Bearer mai_live_your_key\n# or\nX-Api-Key: mai_live_your_key",
      },
    },
    { type: "h2", text: "Key format and storage" },
    {
      type: "list",
      items: [
        "Keys start with `mai_live_` followed by 32 random characters. The fixed prefix makes leaked keys easy to detect with secret scanning.",
        "The plaintext key is shown once, when it is created. The server stores only a SHA-256 hash, so a lost key cannot be recovered: create a new one.",
        "Treat a key like a password: keep it in environment variables or a secret manager, never in client side code or a public repository.",
      ],
    },
    { type: "h2", text: "Errors" },
    {
      type: "table",
      head: ["Status", "Code", "Meaning"],
      rows: [
        ["401", "`missing_authorization`", "No key in `Authorization` or `X-Api-Key`."],
        ["401", "`unknown_api_key`", "The key is malformed, revoked or does not exist."],
        ["401", "`api_key_expired`", "The key passed its `expires_at`. Create a new one."],
        ["403", "`missing_permission`", "The key is scoped and lacks the permission the endpoint needs."],
        ["403", "`permission_escalation`", "A key tried to create or revoke a key with permissions it does not hold."],
        ["429", "`rate_limit_exceeded`", "Too many requests, or too many failed authentications from the same address. Wait for `Retry-After`."],
      ],
    },
    { type: "h2", text: "Scoped keys" },
    {
      type: "p",
      text: "A key either has full access or a list of permissions. Give each agent a key with only what it needs: a triage agent that reads mail and sets labels does not need `message_send`. Keys can also carry an `expires_at`, and revocation takes effect immediately.",
    },
    {
      type: "table",
      head: ["Permission", "Allows"],
      rows: apiReference.permissions.map((p) => [`\`${p}\``, permissionMeaning[p] ?? ""]),
    },
    {
      type: "list",
      items: [
        "A key can create keys only with permissions it holds itself, and a scoped key cannot create a full access key. Violations return `403 permission_escalation`.",
        "Omit `permissions` when creating a key to copy the calling key's permissions.",
        "The same permissions apply to MCP tools, so a scoped key limits what a coding agent can do too.",
      ],
    },
    {
      type: "code",
      sample: {
        label: "Create a read only key for a triage agent",
        lang: "bash",
        code: `curl -s -X POST ${API_URL}/v0/api-keys \\
  -H "Authorization: Bearer $MAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"name": "triage-agent", "permissions": ["inbox_read", "message_read", "message_update"], "expires_at": "2027-01-31T00:00:00Z"}'`,
      },
    },
    { type: "h2", text: "Audit log" },
    {
      type: "p",
      text: "Security relevant actions of your organization, including actions taken by agents through MCP, are recorded. Read them with `GET /v0/audit-logs` (requires `audit_read`).",
    },
    { type: "h2", text: "Endpoints" },
    { type: "endpoints", keys: ["GET /v0/auth/me", "POST /v0/api-keys", "GET /v0/api-keys", "DELETE /v0/api-keys/{key}", "GET /v0/audit-logs"] },
  ],
};
