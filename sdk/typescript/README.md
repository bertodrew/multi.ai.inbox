# @aimulti/inbox

TypeScript client for the multi.ai.inbox API: email inboxes for AI agents,
hosted in the EU. Fetch based, zero runtime dependencies, no build step.

The package ships TypeScript source. Install it as a dependency and let your
bundler, `tsx`, `ts-node` or `vite` compile it, or copy `src/` into a project
that already compiles TypeScript.

```
npm install @aimulti/inbox
```

## Use

```ts
import { AimultiClient, isCode, AimultiError } from "@aimulti/inbox";

const client = new AimultiClient(process.env.MAI_API_KEY!);

const me = await client.me();
const inbox = await client.createInbox({ username: "support-agent", client_id: "support-agent" });
const { threads } = await client.listThreads(inbox.inbox_id, { limit: 10 });
const { message_id } = await client.send(
  inbox.inbox_id,
  { to: ["person@example.com"], subject: "Invoice approved", text: "It will be paid this week." },
  "invoice-42-approval", // Idempotency-Key, so a retry cannot send twice
);
```

Staging and self hosted deployments:

```ts
const client = new AimultiClient(key, { baseUrl: "https://api.staging.example.eu" });
```

## Surface

One method per operation in `openapi/openapi.yaml`:

| Area | Methods |
| --- | --- |
| Identity | `me` |
| Sessions | `requestMagicLink`, `refreshSession`, `logout`, `oauthStart` |
| Inboxes | `createInbox`, `listInboxes`, `getInbox`, `updateInbox`, `deleteInbox` |
| Threads | `listThreads`, `getThread`, `getInboxThread` |
| Messages | `listMessages`, `getMessage`, `updateMessageLabels`, `rawMessage`, `getAttachment`, `send`, `reply`, `releaseMessage` |
| Drafts | `createDraft`, `listDrafts`, `getDraft`, `updateDraft`, `deleteDraft`, `sendDraft` |
| Events | `events` |
| Domains | `createDomain`, `listDomains`, `getDomain`, `deleteDomain`, `verifyDomain` |
| Webhooks | `createWebhook`, `listWebhooks`, `deleteWebhook` |
| Keys | `createApiKey`, `listApiKeys`, `revokeApiKey` |
| Audit | `listAuditLogs` |
| Billing | `billing`, `createCheckout`, `createPortal` |
| Data rights | `exportOrganization`, `deleteOrganization` |
| Support | `listTickets`, `createTicket`, `ticketStats`, `getTicket`, `updateTicket`, `addTicketNote`, `replyTicket` |

Types are hand written from the spec and use the API's own field names, so a
response can be read without a mapping layer. Enums are string literal unions:
`Plan`, `Role`, `EventType`, `Permission`, `TicketState`, `TicketPriority`.

## Errors

A 4xx or 5xx response throws `AimultiError` with the status, the error code, the
request id to quote to support and, when the API sent one, `retryAfter` in
seconds.

```ts
import { AimultiError, isCode } from "@aimulti/inbox";

try {
  await client.createInbox({ username: "support" });
} catch (err) {
  if (isCode(err, "already_exists")) {
    // the address is taken
  } else if (err instanceof AimultiError) {
    console.error(err.status, err.code, err.requestId);
    if (err.retryAfter) await new Promise((r) => setTimeout(r, err.retryAfter! * 1000));
  }
}
```

## Events

`events` is an async generator over server sent events. Leaving the loop closes
the connection.

```ts
for await (const event of client.events({ types: ["message.received"] })) {
  if (event.envelope) {
    console.log(event.id, event.envelope.event_type, event.envelope.data);
  }
}
```

## Attachments

`send`, `reply` and the draft methods take attachments as base64 strings, which
is what the API expects:

```ts
const content = Buffer.from(bytes).toString("base64");
await client.send(address, {
  to: ["person@example.com"],
  subject: "Report",
  attachments: [{ filename: "report.pdf", content_type: "application/pdf", content }],
});
```

`getAttachment` returns a `DownloadedFile` with `filename`, `contentType` and
`data` as a `Uint8Array`.

## Versions

The client speaks `/v1`, the current version. `/v0` is an alias of the same
behaviour and is deprecated: its responses carry `Sunset` and
`Link: </v1/...>; rel="successor-version"`, and its bodies are identical. Pass
`{ version: "v0" }` only for a deployment that predates `/v1`. The deprecation
policy is on the docs page "Versions and SDKs".
