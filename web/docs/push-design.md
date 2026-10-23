# Push notifications: the endpoint this client needs

Status: client side only. The console can ask for permission and register a
subscription, but this build ships with no VAPID key and no subscription route,
so `pushConfigured()` is false and the button on the console home is disabled
with the label "Notifications not configured". No request is made to an endpoint
that does not exist.

This note is the contract to implement on the server. It is deliberately not in
`openapi/openapi.yaml` yet: the route does not exist, and the console's build
time check (`src/lib/console/spec-check.ts`) fails when the spec and the
hand written client disagree. Add the spec first, then the Go handler, then run
`npm run gen`.

## What is already built

* `public/sw.js` handles `push` and renders the notification with
  `showNotification`, and `notificationclick` opens or focuses `/console/`.
* `src/lib/push/client.ts` holds the subscribe and unsubscribe logic, the
  base64url decoding of the VAPID key, and the state machine the UI reads.
* `src/components/console/PushSettings.tsx` is the card on the console home.

## Build variables

Both must be set for the button to do anything. With either missing the UI stays
disabled, which is the state of the current build.

| Variable | Value |
| --- | --- |
| `NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY` | public half of the VAPID key pair, base64url |
| `NEXT_PUBLIC_MAI_PUSH_ENDPOINT` | absolute URL, or a path resolved against the API base the console is signed in to |

The VAPID private key never reaches the browser and should live in the API
deployment's secret store, not in this repo.

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
  organizations.
* `expiration_time` is milliseconds since the epoch, or null.
* Repeat calls with the same `endpoint` must upsert, not duplicate: a returning
  device re-subscribes on every fresh permission grant.

Response `201 Created`:

```json
{ "push_subscription_id": "ps_01J...", "created_at": "2026-10-24T09:12:00Z" }
```

Errors: `401` bad or expired key, `403` key lacks the permission, `422`
malformed subscription, `429` rate limited. An `404` or `405` here is what the
client reports as "This API does not accept push subscriptions yet", which is
what happens today.

## Endpoint: DELETE /v0/push/subscriptions

Body: `{ "endpoint": "https://web.push.apple.com/QG..." }`, answered with `204`.
Also accept `?endpoint=` if a proxy strips DELETE bodies. Called before the
browser drops its own subscription, best effort: a failure here must not stop
the unsubscribe.

## Permission

No permission in `openapi.yaml` covers this. Proposal: `notification_write`,
next to `message_read`, required by both routes. Adding it means:

1. add it to `security`/permissions in `openapi/openapi.yaml`,
2. `npm run gen`,
3. add it to `PERMISSIONS` in `src/lib/console/schemas.ts` and to the `meaning`
   map in `KeysView.tsx`, or the build's spec check fails.

A subscription belongs to the organization of the calling key, and to the
individual key that created it. Revoking the key should delete its
subscriptions: a leaked key must not keep waking devices.

## Storage sketch

```sql
create table push_subscriptions (
  push_subscription_id text primary key,
  organization_id      text not null,
  api_key_id           text,
  endpoint             text not null unique,
  p256dh               text not null,
  auth                 text not null,
  user_agent           text,
  created_at           timestamptz not null default now(),
  last_seen_at         timestamptz,
  failure_count        int not null default 0
);
```

Send on `message.received`, after the gatekeeper verdict, never for mail held for
review. Drop a subscription on `404` or `410` from the push service, and after a
few consecutive failures.

## Payload, and what it must not contain

The service worker renders whatever the payload says, on a lock screen that
anyone can see.

```json
{ "title": "New mail in support@example.com", "body": "Open the console to read it.", "tag": "message.received" }
```

Send the inbox address, never the sender, subject, or a snippet of the body.
`tag` collapses repeat notifications for the same event type.

## Platform limits

* iOS 16.4 and later, and only for an app installed to the home screen: in a
  Safari tab there is no `PushManager` at all. The card says so when it is not
  running standalone.
* The permission prompt must come from a user gesture. `enablePush` calls
  `Notification.requestPermission()` before its first `await` for this reason.
* The service worker must call `showNotification` in the `push` handler, or iOS
  revokes the subscription.
* Payloads over about 4 KB are rejected by Apple.
