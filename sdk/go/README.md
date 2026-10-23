# aimulti, Go client for multi.ai.inbox

A small client for the multi.ai.inbox REST API: email inboxes for AI agents,
hosted in the EU. Standard library only, no dependencies.

```
go get github.com/bertodrew/multi.ai.inbox/sdk/go
```

## Use

```go
import aimulti "github.com/bertodrew/multi.ai.inbox/sdk/go"

client := aimulti.NewClient(os.Getenv("MAI_API_KEY"))

me, err := client.Me(ctx)
inbox, err := client.CreateInbox(ctx, aimulti.CreateInboxRequest{Username: "support-agent", ClientID: "support-agent"})
threads, err := client.ListThreads(ctx, inbox.InboxID, aimulti.ListOptions{Limit: 10})
```

A complete program, compiled with the package, is in `example/main.go`.

## Surface

One method per operation in `openapi/openapi.yaml`, grouped by file:

| Area | Methods |
| --- | --- |
| Identity | `Me` |
| Sessions | `RequestMagicLink`, `RefreshSession`, `Logout`, `OAuthStart` |
| Inboxes | `CreateInbox`, `ListInboxes`, `GetInbox`, `UpdateInbox`, `DeleteInbox` |
| Threads | `ListThreads`, `GetThread`, `GetInboxThread` |
| Messages | `ListMessages`, `GetMessage`, `UpdateMessageLabels`, `RawMessage`, `GetAttachment`, `Send`, `Reply`, `ReleaseMessage` |
| Drafts | `CreateDraft`, `ListDrafts`, `GetDraft`, `UpdateDraft`, `DeleteDraft`, `SendDraft` |
| Events | `Events` (server sent events, `Next` and `Close`) |
| Domains | `CreateDomain`, `ListDomains`, `GetDomain`, `DeleteDomain`, `VerifyDomain` |
| Webhooks | `CreateWebhook`, `ListWebhooks`, `DeleteWebhook` |
| Keys | `CreateAPIKey`, `ListAPIKeys`, `RevokeAPIKey` |
| Audit | `ListAuditLogs` |
| Billing | `Billing`, `CreateCheckout`, `CreatePortal` |
| Data rights | `Export`, `DeleteOrganization` |
| Support | `ListTickets`, `CreateTicket`, `TicketStats`, `GetTicket`, `UpdateTicket`, `AddTicketNote`, `ReplyTicket` |

Every method takes a `context.Context` first. Values are the API's own shapes,
so a field in the reference is a field in a struct.

## Options

| Option | Effect |
| --- | --- |
| `WithBaseURL(url)` | Another deployment, for example a staging host |
| `WithHTTPClient(*http.Client)` | A proxy, a timeout or a traced transport |
| `WithUserAgent(string)` | The `User-Agent` header |
| `WithAPIVersion("v0")` | Speak the deprecated prefix, for a deployment that predates `/v1` |

Per call, `WithIdempotencyKey(key)` makes `Send`, `Reply`, `SendDraft` and
`ReplyTicket` safe to retry.

## Errors

A 4xx or 5xx response is a `*aimulti.Error` with the HTTP status, the error
envelope (`Name`, `Code`, `Message`), the request id to quote to support and
`RetryAfter` when the API sent one.

```go
var apiErr *aimulti.Error
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Code, apiErr.RequestID)
}
if aimulti.IsCode(err, "already_exists") {
	// the inbox address is taken
}
if wait := aimulti.RetryAfter(err); wait > 0 {
	time.Sleep(wait)
}
```

## Versions

The client speaks `/v1`. `/v0` is an alias of the same behaviour and is
deprecated: its responses carry `Sunset` and
`Link: </v1/...>; rel="successor-version"`. Nothing in a body differs between
the two, so moving a client is `WithAPIVersion` away, or nothing at all when the
default is already `v1`. The deprecation policy is on the docs page "Versions
and SDKs".

## Notes

* Path segments (an inbox address, a Message-ID, an id) are escaped; pass the
  raw value.
* `Attachment` takes `Data []byte` and encodes it as the base64 string the API
  expects.
* `UpdateDraftRequest.CancelSendAt` sends `send_at: null`, which unschedules a
  scheduled draft.
* `Events` returns a stream: call `Next` in a loop and `Close` when done. `Close`
  unblocks a pending `Next`.
