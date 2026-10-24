# Push notifications: the endpoint this client needs

Status: built. `internal/db/migrations/0014_push.sql` stores the subscriptions,
`internal/core/push.go` validates them, queues the notification on
`message.received` and sends it with VAPID, `internal/api/push.go` serves the
routes, and the console card is wired to them. Push is off until the deployment
sets a VAPID key pair, exactly like billing: every route then answers
`501 push_disabled` and the card stays disabled with the label "Notifications
not configured". [docs/push-enable.md](../../docs/push-enable.md) is the
operator guide.

This note is the contract the server keeps. Anything below that the code does
not do is a bug in the code.

## What is built

* `public/sw.js` handles `push` and renders the notification with
  `showNotification`, and `notificationclick` opens or focuses the deep link in
  the payload, or `/console/` when there is none. A payload can only point at a
  path on the console origin.
* `src/lib/push/client.ts` holds the subscribe and unsubscribe logic, the
  base64url decoding of the VAPID key, and the state machine the UI reads. It
  calls the same typed API client as the rest of the console.
* `src/components/console/PushSettings.tsx` is the card on the console home: it
  turns notifications on and off for this device and lists the devices this key
  registered, each with a Remove button.
* `POST`, `GET` and `DELETE /v0/push/subscriptions` are in
  `openapi/openapi.yaml`, which `npm run gen` turns into the console's typed
  reference, and `src/lib/console/spec-check.ts` fails the build when the two
  drift.

## Build variable

| Variable | Value |
| --- | --- |
| `NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY` | public half of the VAPID key pair, base64url |

That one variable is the whole client configuration: with it set the card can
subscribe, without it the card says the build has no push key. The route lives
on the API base the console is already signed in to, so there is no second URL
and no second origin to allow in the content security policy.

The VAPID private key never reaches the browser and lives in the API
deployment's secret store (`MAI_VAPID_KEYS`), not in this repo.

## Endpoint: POST /v0/push/subscriptions

Called by the signed in console, from the browser, with the same key the rest of
the console uses.

Headers: `Authorization: Bearer <api key>`, `Content-Type: application/json`.

```json
{
  "endpoint": "https://web.push.apple.com/QG...",
  "keys": { "p256dh": "BNc...", "auth": "k9U..." },
  "expiration_time": null,
  "user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)"
}
```

* `endpoint` identifies the device at its push service. It is a capability: do
  not log it, do not return it to any client, and do not share it across
  organizations. The table keeps one row per organization and endpoint.
* `keys.p256dh` and `keys.auth` are capabilities too. They are validated (an
  uncompressed P-256 point and a 16 byte secret), stored as the browser produced
  them, and never returned, logged or audited.
* `expiration_time` is milliseconds since the epoch, or null.
* Repeat calls with the same `endpoint` upsert, not duplicate: a returning
  device re-subscribes on every fresh permission grant. The row keeps its
  original `created_at` and takes the new keys and user agent.

Response `201 Created`:

```json
{
  "push_subscription_id": "5f0d1c1e-4f1a-4d0f-9c35-8a2c0e1b7d21",
  "device": "iPhone",
  "user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)",
  "created_at": "2026-10-24T09:12:00Z"
}
```

`device` is a label derived from the user agent, not an identity.

Errors: `401` bad or expired key, `403` key lacks the permission, `400`
malformed subscription (a field the server does not know is refused), `409`
`too_many_subscriptions` when the organization already has 50 devices, `501`
`push_disabled` when the deployment has no VAPID key pair.

## Endpoint: GET /v0/push/subscriptions

What the caller registered, newest first: `push_subscription_id`, `device`,
`user_agent`, `created_at`, `last_used_at`. Never the endpoint, never the keys.

## Endpoint: DELETE /v0/push/subscriptions/{id}

Answered with `204`. `404` when this organization has no such subscription.
`DELETE /v0/push/subscriptions` with `{ "endpoint": "https://..." }` in the body
or `?endpoint=` in the query does the same for a client that only knows the
endpoint, and answers `204` even when the endpoint is unknown: the browser has
already dropped it. Called before the browser drops its own subscription, best
effort: a failure here must not stop the unsubscribe.

## Permission

`notification_write` is what it takes to register or drop a device. It is in
`AllPermissions` and `GrantablePermissions`, and `PermissionsForRole` grants it
to member, admin and owner, because a person registering their own phone is not
an authorization decision. It reveals nothing about the mail: a key without
`message_read` still cannot read what it was notified about.

Both changing routes are audited (`push.subscribe`, `push.unsubscribe`), with
the device label and never the endpoint or the keys. The list is a read and is
not audited, like every other list in the console.

A subscription belongs to the organization of the calling key, and to the
individual key or person that created it. Revoking the key deletes its
subscriptions: a leaked key must not keep waking devices.

## Storage

`push_subscriptions` holds the device; `push_jobs` is the delivery queue. Both
carry `org_id` and the same forced row level security as every other tenant
table (`app_org()` / `app_system()`).

```sql
create table push_subscriptions (
  id            uuid primary key default gen_random_uuid(),
  org_id        uuid not null references organizations(id) on delete cascade,
  user_id       uuid references users(id) on delete cascade,
  api_key_id    uuid references api_keys(id) on delete cascade,
  endpoint      text not null,
  p256dh        text not null,
  auth          text not null,
  user_agent    text not null default '',
  created_at    timestamptz not null default now(),
  last_used_at  timestamptz,
  failure_count int not null default 0,
  unique (org_id, endpoint),
  check (user_id is not null or api_key_id is not null)
);
```

A job is written in the transaction that stores the message, after the event is
written, and never for mail held for review: `message.received` is the trigger,
so a released message notifies too, and spam does not. Ingest makes no HTTP
call, and a push problem is logged and dropped rather than failing the delivery.

One pending job per subscription and inbox: a burst collapses. A device is
notified at most once every 30 seconds. A `404` or `410` from the push service
removes the subscription at once; five consecutive failures retire it; a
temporary failure is retried with a growing delay, up to five attempts. Finished
jobs are purged after seven days.

## Payload, and what it must not contain

The service worker renders whatever the payload says, on a lock screen that
anyone can see.

```json
{
  "title": "New mail in support@example.com",
  "body": "Open the console to read it.",
  "tag": "message.received",
  "url": "/console/inbox/?address=support%40example.com&thread=<thread uuid>",
  "data": { "kind": "message.received", "inbox": "support@example.com", "thread_id": "<uuid>", "ticket_number": 42 }
}
```

The inbox address, the thread id (a platform uuid) and the support ticket number
are the whole content. Never the sender, never the subject, never a snippet of
the body: a subject on a lock screen is the mail, and the first version of this
note said so before it said anything else. A thread with a ticket says so in the
body: `Ticket #42. Open the console to read it.` `tag` collapses repeat
notifications for the same event type. The payload stays well under the 4 KB
Apple rejects.

## Platform limits

* iOS 16.4 and later, and only for an app installed to the home screen: in a
  Safari tab there is no `PushManager` at all. The card says so when it is not
  running standalone.
* The permission prompt must come from a user gesture. `enablePush` calls
  `Notification.requestPermission()` before its first `await` for this reason.
* The service worker must call `showNotification` in the `push` handler, or iOS
  revokes the subscription.
* Payloads over about 4 KB are rejected by Apple.
