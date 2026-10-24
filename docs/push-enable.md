# Enabling web push

Web push is built and off by default, the same way billing is: without a VAPID
key pair the API starts normally, logs `web push disabled`, answers
`501 push_disabled` on every `/v0/push` route, and the console card stays
disabled with the label "Notifications not configured".

The contract the server implements is [web/docs/push-design.md](../web/docs/push-design.md);
the routes are in `openapi/openapi.yaml`; the code is `internal/core/push.go`,
`internal/api/push.go` and `internal/db/migrations/0014_push.sql`.

## 1. Generate the key pair, once

```sh
mai admin vapid-keygen > vapid.json
```

It prints one JSON document and writes the warning to stderr: the pair is shown
once. Generate it on a machine you trust, put it in the secret store, and delete
the file. Never commit it, never paste it into a chat or a ticket, never log it.

Rotating the key pair invalidates every subscription the platform stores: a
browser subscribed to the old key gets a 403 from its push service and is
retired after a few failed sends. Push the new public key to the console build
in the same release.

## 2. Give the API the private half

Both halves live in one variable so the secret stays a single value:

```sh
kubectl -n mai create secret generic mai-secrets \
  --from-literal=MAI_VAPID_KEYS="$(cat vapid.json)" \
  --dry-run=client -o yaml | kubectl apply -f -
```

`MAI_VAPID_SUBJECT` is a contact for the push service to complain to, not a
secret; put it in the `mai-config` ConfigMap. It must be an `https://` URL or an
email address.

The two halves may also be given separately, which is what some secret managers
prefer:

| Variable | Value |
| --- | --- |
| `MAI_VAPID_PRIVATE_KEY` | the 32 byte P-256 scalar, base64url |
| `MAI_VAPID_PUBLIC_KEY` | the matching uncompressed point, base64url |
| `MAI_VAPID_SUBJECT` | `mailto:ops@your-domain` or an https URL |

A public key that does not belong to the private key is refused at start up,
because every notification would fail at the push service.

## 3. Build the console with the public half

```sh
NEXT_PUBLIC_MAI_VAPID_PUBLIC_KEY="$(jq -r .public_key vapid.json)" npm run build
```

Only the public half reaches the browser. The console keeps talking to the API
base it is already signed in to, so there is no second URL to configure.

## 4. Check it

```sh
mai admin push-status
```

It reads the environment only, never prints key material, and says what is
missing:

```json
{
  "enabled": true,
  "public_key": "BL...",
  "subject": "mailto:ops@your-domain",
  "warnings": []
}
```

The `push` role (part of the default `mai serve` roles) drains the queue. Without
it, subscriptions are stored and audited, notifications pile up, and nothing is
delivered.

## What a device sees

A notification names the inbox and links into the console:

```
New mail in support@example.com
Open the console to read it.
```

A support thread adds its number: `Ticket #42. Open the console to read it.` The
payload never carries the sender, the subject, a preview or a line of the body:
it is rendered on a lock screen anyone standing nearby can read.

Delivery rules, all in `internal/core/push.go`:

* A burst of mail to the same inbox collapses into one pending notification per
  device, and a device is notified at most once every 30 seconds.
* A device the push service reports gone (404 or 410) is removed at once.
* Five consecutive failures retire a device. A temporary failure is retried
  with a growing delay, up to five attempts.
* Ingest never waits for a push service and never fails because of one: the
  queue row is written in the same transaction as the message, and a push
  problem is logged and dropped.
* A subscription is bound to the organization and to the key or person that
  registered it. Revoking the key deletes its subscriptions.

## Platform limits

iOS 16.4 and later delivers web push only to a console added to the home screen
and opened from there; in a Safari tab there is no `PushManager` at all, and the
console card says so. The permission prompt comes from a user gesture, which is
why the button asks directly. Payloads stay far under the 4 KB Apple rejects.
