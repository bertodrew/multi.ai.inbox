import { API_URL } from "@/lib/site";
import type { DocPage } from "../types";

export const inboxes: DocPage = {
  slug: "inboxes",
  title: "Inboxes",
  description: "Create, list, rename and delete inboxes. One inbox per agent, per user or per task.",
  blocks: [
    {
      type: "p",
      text: "An inbox is an email address your agent owns. Creating one is a single API call; there is nothing to configure on a mail server.",
    },
    { type: "h2", text: "Create" },
    {
      type: "list",
      items: [
        "`username`: the local part, lowercase letters, digits, dots, dashes and underscores. Random when empty.",
        "`domain`: a verified [custom domain](/docs/custom-domains) of your organization. The platform shared domain when empty.",
        "`display_name`: the name shown to recipients, up to 200 characters.",
        "`client_id`: your own identifier. Repeating it returns the existing inbox with status `200` instead of `201`, so create calls are safe to retry.",
      ],
    },
    {
      type: "code",
      sample: {
        label: "cURL",
        lang: "bash",
        code: `curl -s -X POST ${API_URL}/v0/inboxes \\
  -H "Authorization: Bearer $MAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"username": "billing", "domain": "agents.your-domain.eu", "display_name": "Billing agent", "client_id": "billing"}'`,
      },
    },
    {
      type: "p",
      text: "Errors: `403 limit_exceeded` when the plan's inbox limit is reached, `403 domain_not_verified` when the domain is not verified for your organization, `409 already_exists` when the address is taken, `409 address_quarantined` when it belonged to an inbox deleted less than 90 days ago.",
    },
    { type: "h2", text: "Identify an inbox" },
    {
      type: "p",
      text: "Paths take the inbox email address as `{inbox}`, for example `/v0/inboxes/billing@agents.your-domain.eu`. URL encode it when your HTTP client does not do it for you.",
    },
    { type: "h2", text: "Delete" },
    {
      type: "p",
      text: "Deleting an inbox deletes all its threads, messages and attachments, including the stored raw messages and files. It cannot be undone. The address is then reserved for 90 days: no organization can create it again in that window (`409 address_quarantined`), so nobody receives mail meant for the previous owner.",
    },
    { type: "h2", text: "Endpoints" },
    {
      type: "endpoints",
      keys: ["POST /v0/inboxes", "GET /v0/inboxes", "GET /v0/inboxes/{inbox}", "PATCH /v0/inboxes/{inbox}", "DELETE /v0/inboxes/{inbox}"],
    },
    { type: "schemas", names: ["Inbox"] },
  ],
};

export const messagesAndThreads: DocPage = {
  slug: "messages-and-threads",
  title: "Messages and threads",
  description: "How inbound mail is parsed, threaded, labeled and screened before your agent reads it.",
  blocks: [
    {
      type: "p",
      text: "Inbound mail is parsed into **messages** and grouped into **threads** using the `In-Reply-To` and `References` headers. Your agent reads conversations, not raw MIME.",
    },
    { type: "h2", text: "Threads" },
    {
      type: "p",
      text: "List threads of one inbox, or across the whole organization with `GET /v0/threads`. Lists are newest first and paginated with `limit` (1 to 100, default 20) and `page_token` (from `next_page_token`). `GET /v0/threads/{thread}` returns the thread with all messages, oldest first.",
    },
    { type: "h2", text: "Messages" },
    {
      type: "p",
      text: "Listing returns metadata without bodies, to keep responses small for agents. Getting one message returns:",
    },
    {
      type: "list",
      items: [
        "`text` and `html`: the bodies as sent.",
        "`extracted_text`: only the new text, with quoted history and mobile signatures removed. Set by the gatekeeper; prefer it when you pass a message to a model.",
        "`authentication_results`: DKIM result for the From domain, spam score and spam action.",
        "`gatekeeper`: prompt injection screening, see below.",
        "`attachments`: id, filename, content type and size. See [Attachments](/docs/attachments).",
      ],
    },
    {
      type: "p",
      text: "The original message is available as RFC 822 from `GET .../messages/{message}/raw`. The `{message}` path parameter accepts the message UUID or the RFC `Message-ID` (URL encoded).",
    },
    { type: "h2", text: "Gatekeeper verdict" },
    {
      type: "p",
      text: "Every inbound message is screened for prompt injection before it is marked complete. The verdict is `clear` or `review`, with a score and the reasons that triggered it, such as `instruction_override`, `data_exfiltration`, `role_hijack`, `fake_tool_markup`, `hidden_html_text`, `invisible_unicode` or `dmarc_fail`.",
    },
    {
      type: "code",
      sample: {
        label: "gatekeeper field",
        lang: "json",
        code: `"gatekeeper": {
  "status": "complete",
  "verdict": "review",
  "score": 0.8,
  "reasons": ["instruction_override", "data_exfiltration"],
  "model": "gatekeeper-heuristics-v1"
}`,
      },
    },
    {
      type: "note",
      tone: "warn",
      text: "A `clear` verdict lowers risk, it does not remove it. Keep sensitive actions (payments, sharing credentials, forwarding mail) behind a confirmation step in your agent, whatever the verdict.",
    },
    {
      type: "p",
      text: "`status` is `pending` until screening finishes, `unavailable` if the screener could not run and `timeout` if no verdict arrived within 15 minutes. Messages with verdict `review` also get the `review` label, and the `message.security.review` event fires.",
    },
    { type: "h2", text: "Screen before notify" },
    {
      type: "p",
      text: "Every organization has the setting `screen_before_notify`, on by default (see `GET /v0/auth/me`). Inbound mail is held until the gatekeeper verdict exists, so an agent never reads a prompt injection before screening finishes. The message field `screening` shows the hold state:",
    },
    {
      type: "table",
      head: ["`screening`", "Meaning", "Visible", "Events"],
      rows: [
        ["(pending)", "Waiting for the gatekeeper.", "Never returned.", "None yet."],
        ["`allowed`", "Verdict `clear`.", "Yes.", "`message.security.completed`, then `message.received`."],
        ["`review`", "Verdict `review`, screener unavailable or timed out: the hold fails closed.", "Only with `include_review=true` and `message_review`.", "`message.security.review` only, without bodies."],
        ["`released`", "A reviewer released it.", "Yes.", "`message.received`."],
      ],
    },
    {
      type: "p",
      text: "Held messages are left out of message and thread lists, gets, raw downloads and attachments, and cannot be replied to or relabeled. A thread started by a held message stays hidden until one of its messages is visible. The same rules apply to the MCP tools.",
    },
    {
      type: "code",
      sample: {
        label: "Review and release",
        lang: "bash",
        code: `# a key with message_read and message_review
curl -s "${API_URL}/v0/inboxes/billing@agents.your-domain.eu/messages?include_review=true&label=review" \
  -H "Authorization: Bearer $MAI_REVIEW_KEY"

# a key with message_release: the message becomes visible and message.received fires
curl -s -X POST "${API_URL}/v0/inboxes/billing@agents.your-domain.eu/messages/$MESSAGE_ID/release" \
  -H "Authorization: Bearer $MAI_REVIEW_KEY"`,
      },
    },
    {
      type: "note",
      tone: "warn",
      text: "Give `message_review` and `message_release` to a human review tool, not to the agent that reads the inbox: an agent that can release its own mail can be talked into it. Every release is written to the audit log. Keys without a permission list hold every permission, including these two, so give agents scoped keys.",
    },
    { type: "h2", text: "Labels" },
    {
      type: "p",
      text: "The platform sets system labels: `received` and `unread` on inbound mail, `sent` on outbound mail, `spam` when the spam filter flags a message, `review` when the gatekeeper does. Add your own labels to track workflow state, and filter any list with `?label=`.",
    },
    {
      type: "code",
      sample: {
        label: "Update labels",
        lang: "bash",
        code: `curl -s -X PATCH "${API_URL}/v0/inboxes/billing@agents.your-domain.eu/messages/$MESSAGE_ID" \\
  -H "Authorization: Bearer $MAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"add_labels": ["invoice", "approved"], "remove_labels": ["unread"]}'`,
      },
    },
    { type: "h2", text: "Endpoints" },
    {
      type: "endpoints",
      keys: [
        "GET /v0/inboxes/{inbox}/threads",
        "GET /v0/threads",
        "GET /v0/threads/{thread}",
        "GET /v0/inboxes/{inbox}/messages",
        "GET /v0/inboxes/{inbox}/messages/{message}",
        "PATCH /v0/inboxes/{inbox}/messages/{message}",
        "GET /v0/inboxes/{inbox}/messages/{message}/raw",
        "POST /v0/inboxes/{inbox}/messages/{message}/release",
      ],
    },
    { type: "schemas", names: ["Thread", "Message"] },
  ],
};

export const sendingAndReplying: DocPage = {
  slug: "sending-and-replying",
  title: "Sending and replying",
  description: "Send new messages, reply in thread and retry safely with Idempotency-Key.",
  blocks: [
    { type: "h2", text: "Send a new message" },
    {
      type: "p",
      text: "`POST /v0/inboxes/{inbox}/messages/send` starts a new thread. `to` needs at least one address; `text` or `html` is required. The response is `200` with the `message_id` and `thread_id` once the message is queued; delivery happens in the background and ends with a `message.sent` or `message.failed` event. When more than 5% of an inbox's (or organization's) last 24 hours of sends hard bounced, over at least 20 sends, sending is paused: the call returns `403 sending_paused` until support resumes it.",
    },
    {
      type: "code",
      sample: {
        label: "cURL",
        lang: "bash",
        code: `curl -s -X POST "${API_URL}/v0/inboxes/billing@agents.your-domain.eu/messages/send" \\
  -H "Authorization: Bearer $MAI_API_KEY" \\
  -H "Content-Type: application/json" \\
  -H "Idempotency-Key: order-1042-confirmation" \\
  -d '{"to": ["customer@example.com"], "subject": "Order 1042", "text": "Your order has shipped."}'`,
      },
    },
    { type: "h2", text: "Reply" },
    {
      type: "p",
      text: "`POST /v0/inboxes/{inbox}/messages/{message}/reply` answers in the same thread. Recipients, subject, `In-Reply-To` and `References` are set for you. Set `reply_all` to include the other recipients.",
    },
    { type: "h2", text: "Idempotency-Key" },
    {
      type: "p",
      text: "Agents retry. To make retries safe, send an `Idempotency-Key` header (up to 256 characters) on send and reply:",
    },
    {
      type: "list",
      items: [
        "The same key with the same request within 24 hours returns the stored result and sends nothing new.",
        "The same key with a different request returns `409 conflict`.",
        "While the first request with a key is still running, a second one returns `409 conflict`: retry to get the result.",
      ],
    },
    { type: "p", text: "Derive the key from your own state (an order id, a task id, the id of the message you answer), not from a random value generated at retry time." },
    { type: "h2", text: "Delivery" },
    {
      type: "p",
      text: "Messages are DKIM signed with your domain's key. Mail to another inbox on the platform is delivered locally (agent to agent) without leaving the system. Other mail currently goes out through an EU SMTP relay. Failed attempts are retried after 1, 5 and 30 minutes, then 2 and 6 hours, before the message is marked `failed`.",
    },
    { type: "h2", text: "Limits" },
    {
      type: "list",
      items: [
        "Up to 50 recipients per message across `to`, `cc` and `bcc`.",
        "Daily and monthly send limits depend on the plan; going over returns `429 limit_exceeded`. See [Limits](/docs/limits).",
        "On the free plan a short footer is appended: \"Sent via multi.ai.inbox\".",
      ],
    },
    { type: "h2", text: "Drafts" },
    {
      type: "p",
      text: "A draft is a message saved for later: for a human to review, or to send at `send_at`. Create it with the same fields as a send; recipients and a body are only needed when it is sent. With `in_reply_to` (a message id) the draft is a reply: `to`, `subject` and the threading headers default to those of that message, and the sent message joins its thread. `POST .../drafts/{draft}/send` sends it now through the normal send path (plan limits, `403 sending_paused`, `Idempotency-Key`) and deletes the draft.",
    },
    {
      type: "list",
      items: [
        "`send_at` schedules the draft; the scheduler checks every minute. `PATCH` with `send_at: null` cancels it.",
        "If a scheduled send is refused (for example a plan limit), the draft gets `status: failed` and `last_error`. Edit it or send it by hand to retry.",
        "Creating a draft needs `message_send`; reading drafts needs `message_read`. `draft.created` fires for every new draft.",
      ],
    },
    { type: "h2", text: "Endpoints" },
    {
      type: "endpoints",
      keys: [
        "POST /v0/inboxes/{inbox}/messages/send",
        "POST /v0/inboxes/{inbox}/messages/{message}/reply",
        "POST /v0/inboxes/{inbox}/drafts",
        "GET /v0/inboxes/{inbox}/drafts",
        "GET /v0/inboxes/{inbox}/drafts/{draft}",
        "PATCH /v0/inboxes/{inbox}/drafts/{draft}",
        "DELETE /v0/inboxes/{inbox}/drafts/{draft}",
        "POST /v0/inboxes/{inbox}/drafts/{draft}/send",
      ],
    },
    { type: "schemas", names: ["Send", "Reply", "SendResult", "Draft"] },
  ],
};

export const attachments: DocPage = {
  slug: "attachments",
  title: "Attachments",
  description: "Send attachments as base64 and download received ones.",
  blocks: [
    { type: "h2", text: "Sending" },
    {
      type: "p",
      text: "Add `attachments` to a send or reply body. Each item has `filename`, `content_type` and `content` encoded as base64. Attachments of one message can total up to 6 MB; larger requests return `413 payload_too_large`.",
    },
    {
      type: "code",
      sample: {
        label: "Attachment in a send body",
        lang: "json",
        code: `{
  "to": ["accounts@example.com"],
  "subject": "Invoice 2026-118",
  "text": "Invoice attached.",
  "attachments": [
    { "filename": "invoice-2026-118.pdf", "content_type": "application/pdf", "content": "JVBERi0xLjcK..." }
  ]
}`,
      },
    },
    { type: "h2", text: "Receiving" },
    {
      type: "p",
      text: "A received message lists its attachments with `attachment_id`, `filename`, `content_type` and `size`. Download the bytes with `GET .../messages/{message}/attachments/{attachment}`; the response carries the original content type.",
    },
    {
      type: "p",
      text: "Over MCP, `get_attachment` returns text files as text and other files as base64, up to 5 MB. Download larger files with the REST endpoint.",
    },
    {
      type: "note",
      tone: "warn",
      text: "Attachments come from outside your system. Open them in a sandbox and do not let a model execute instructions found inside them.",
    },
    { type: "h2", text: "Endpoints" },
    { type: "endpoints", keys: ["GET /v0/inboxes/{inbox}/messages/{message}/attachments/{attachment}"] },
    { type: "schemas", names: ["Attachment"] },
  ],
};
