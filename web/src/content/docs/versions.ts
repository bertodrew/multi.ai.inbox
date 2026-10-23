import { API_URL } from "@/lib/site";
import type { DocPage } from "../types";

export const versions: DocPage = {
  slug: "versions",
  title: "Versions and SDKs",
  description: "How /v1 and /v0 relate, what the deprecation headers mean, and the official clients.",
  blocks: [
    {
      type: "p",
      text: `Base URL: \`${API_URL}\`. Every operation answers on \`/v1\`, the current version, and on \`/v0\`, which is kept as an alias and is deprecated. The two are the same code: same paths below the prefix, same request fields, same response bodies, same status codes, same error codes. There is nothing to negotiate and no header to send: the version lives in the path.`,
    },
    { type: "h2", text: "What each version answers" },
    {
      type: "table",
      head: ["Path", "Status", "Response headers"],
      rows: [
        ["`/v1/...`", "Current version, safe for new integrations", "`Aimulti-Api-Version: v1`"],
        [
          "`/v0/...`",
          "Same behaviour, deprecated, no new features",
          "`Aimulti-Api-Version: v0`, `Sunset: Fri, 31 Dec 2027 00:00:00 GMT`, `Link: </v1/...>; rel=\"successor-version\"`",
        ],
      ],
    },
    {
      type: "p",
      text: "`Aimulti-Api-Version` names the version that answered, on either prefix, so a log or a bug report always says which path a client used. `Sunset` is the last day `/v0` is promised to answer, as an HTTP date. `Link` with `rel=\"successor-version\"` points at the same operation under `/v1`, so a client can follow it without guessing how paths map.",
    },
    {
      type: "code",
      sample: {
        label: "cURL",
        lang: "bash",
        code: `curl -s -D - -o /dev/null "${API_URL}/v0/me" \\
  -H "Authorization: Bearer ***"

# HTTP/2 401
# aimulti-api-version: v0
# sunset: Fri, 31 Dec 2027 00:00:00 GMT
# link: </v1/me>; rel="successor-version"`,
      },
    },
    { type: "h2", text: "Deprecation policy" },
    {
      type: "list",
      items: [
        "Breaking changes ship only in a new version prefix. Within a version we add fields and endpoints, and never rename or remove one, and never change the meaning of a status code or an error code.",
        "A version is supported for at least twelve months after its successor ships. `/v1` shipped in October 2026, so `/v0` answers until 31 December 2027.",
        "The day a successor ships, the old prefix starts sending `Sunset` and `Link` on every response: a 200, a 404, an error envelope, a redirect. Deprecation is never silent and never a surprise in a changelog only.",
        "The deprecation is never in the response body. `/v0` returns exactly the JSON it always returned, so an old client that parses the envelope keeps working, and a warning cannot break a strict parser.",
        "Announcements go to the address in the console account and to the changelog, at least twelve months before a sunset date. The date in the `Sunset` header never moves closer without that notice.",
        "An urgent security fix can change behaviour inside a version. When that happens it applies to both prefixes at once, because they are the same handler, and the changelog says what changed.",
      ],
    },
    {
      type: "note",
      tone: "info",
      text: "There is no separate `/v0` documentation to keep in sync: this site documents the operations once. Where a page shows `curl` against `/v0`, replace the prefix with `/v1` and everything else stays the same.",
    },
    { type: "h2", text: "Moving a client to /v1" },
    {
      type: "list",
      ordered: true,
      items: [
        "Change the base path from `/v0` to `/v1`. Nothing else in a request changes.",
        "Leave headers and bodies alone. `Idempotency-Key`, `include_review`, pagination tokens and error codes all behave as they did.",
        "Read `Aimulti-Api-Version` in logs, and alert if `Sunset` appears: that is a client still on the old prefix.",
        "Watch for a `Location` or `Link` value that already points at `/v1` and use it, rather than rebuilding paths by hand.",
      ],
    },
    { type: "h2", text: "Go SDK" },
    {
      type: "p",
      text: "A small client with no dependencies outside the standard library. One method per operation, a `context.Context` first, typed values that mirror this reference, and an error type carrying the API code and the request id.",
    },
    {
      type: "code",
      sample: {
        label: "Go",
        lang: "go",
        code: `import aimulti "github.com/bertodrew/multi.ai.inbox/sdk/go"

client := aimulti.NewClient(os.Getenv("MAI_API_KEY"))

me, err := client.Me(ctx)
inbox, err := client.CreateInbox(ctx, aimulti.CreateInboxRequest{
    Username: "support-agent", ClientID: "support-agent",
})
threads, err := client.ListThreads(ctx, inbox.InboxID, aimulti.ListOptions{Limit: 10})
result, err := client.Send(ctx, inbox.InboxID, aimulti.SendRequest{
    To: []string{"person@example.com"}, Subject: "Approved", Text: "It ships today.",
}, aimulti.WithIdempotencyKey("approved-42"))`,
      },
    },
    {
      type: "p",
      text: "Options cover the deployment (`WithBaseURL`), the transport (`WithHTTPClient`) and the deprecated prefix (`WithAPIVersion`). Errors are `*aimulti.Error`, with helpers `IsCode`, `StatusCode` and `RetryAfter`. The client speaks `/v1` by default. The module lives in `sdk/go/`, with a README and a compiled example.",
    },
    { type: "h2", text: "TypeScript SDK" },
    {
      type: "p",
      text: "Fetch based, no runtime dependencies, and shipped as TypeScript source, so there is no build step to read it. Hand written types mirror this reference and use the API's own field names.",
    },
    {
      type: "code",
      sample: {
        label: "TypeScript",
        lang: "ts",
        code: `import { AimultiClient, isCode } from "@aimulti/inbox";

const client = new AimultiClient(process.env.MAI_API_KEY!);

const inbox = await client.createInbox({ username: "support-agent", client_id: "support-agent" });
const { threads } = await client.listThreads(inbox.inbox_id, { limit: 10 });

try {
  await client.send(inbox.inbox_id, {
    to: ["person@example.com"], subject: "Approved", text: "It ships today.",
  }, "approved-42");
} catch (err) {
  if (isCode(err, "already_exists")) {
    // handle it
  }
}`,
      },
    },
    {
      type: "p",
      text: "Errors are `AimultiError`, with `status`, `code`, `requestId` and `retryAfter`. `events()` is an async generator over server sent events. The package lives in `sdk/typescript/`, with a README.",
    },
    { type: "h2", text: "Supporting another version" },
    {
      type: "p",
      text: "Neither SDK sends a version header, so pointing one at `/v0` is a single option away, and the default is already `/v1`. If your language is not covered yet, the REST surface is the contract: every operation in the [API reference](/docs/api-reference) works on both prefixes.",
    },
  ],
};
