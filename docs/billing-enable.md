# Enabling Stripe billing

Billing is implemented and wired but **off by default**: with
`MAI_STRIPE_SECRET_KEY` or `MAI_STRIPE_WEBHOOK_SECRET` missing, every billing
endpoint answers `501 billing_disabled` and the API logs `billing disabled` at
startup. No code change is needed to switch it on — only environment.

The whole surface is: `GET /v0/billing`, `POST /v0/billing/checkout`,
`POST /v0/billing/portal` (all permission-gated) and the public
`POST /stripe/webhook`. Plans and limits stay in `organizations.plan`; Stripe
only moves that column.

## 1. Create the six prices in Stripe

One product per plan (Builder, Growth, Team) or a single product with six
prices — the code only cares about price ids. In EUR, **tax behaviour:
exclusive** (Stripe Tax adds VAT on top), recurring:

| Plan | Monthly | Yearly |
| --- | --- | --- |
| builder | 1 price | 1 price (10 × monthly) |
| growth | 1 price | 1 price |
| team | 1 price | 1 price |

Enable **Stripe Tax** and let Checkout collect a VAT id: the code sends
`automatic_tax[enabled]=true`, `tax_id_collection[enabled]=true` and
`billing_address_collection=required`.

## 2. Register the webhook

Endpoint URL: `https://<api host>/stripe/webhook`

Events:

- `checkout.session.completed`
- `customer.subscription.created`
- `customer.subscription.updated`
- `customer.subscription.deleted`
- `invoice.payment_failed`

Copy the endpoint's **signing secret** (`whsec_...`), not the API key.

## 3. Set the environment

| Variable | Secret | Purpose |
| --- | --- | --- |
| `MAI_STRIPE_SECRET_KEY` | yes | Stripe API key (`sk_` or a restricted `rk_`) |
| `MAI_STRIPE_WEBHOOK_SECRET` | yes | Endpoint signing secret (`whsec_`) |
| `MAI_BILLING_RETURN_URL` | no | Default success/cancel/portal return page |
| `MAI_STRIPE_PRICE_BUILDER_MONTHLY` | no | price id |
| `MAI_STRIPE_PRICE_BUILDER_YEARLY` | no | price id |
| `MAI_STRIPE_PRICE_GROWTH_MONTHLY` | no | price id |
| `MAI_STRIPE_PRICE_GROWTH_YEARLY` | no | price id |
| `MAI_STRIPE_PRICE_TEAM_MONTHLY` | no | price id |
| `MAI_STRIPE_PRICE_TEAM_YEARLY` | no | price id |

Both keys are mandatory as a pair: setting only one keeps billing off and the
startup log names the missing variable.

### k3s (staging and production)

Prices and the return URL go in the optional `mai-billing` ConfigMap
(`deploy/k8s/staging/kustomization.yaml` has it commented out, ready to fill).
The two keys go in the existing `mai-secrets` Secret:

```sh
kubectl -n mai patch secret mai-secrets --type merge -p \
  "{\"stringData\":{\"MAI_STRIPE_SECRET_KEY\":\"sk_live_...\",\"MAI_STRIPE_WEBHOOK_SECRET\":\"whsec_...\"}}"
```

Neither the Secret values nor the kustomization literals are ever committed.

### Local (docker compose)

Uncomment the block in `deploy/local/.env.example` into `deploy/local/.env`.

## 4. Verify without charging anyone

```sh
mai admin billing-status
```

Offline: reads the environment only, never calls Stripe, and prints the keys
**redacted** — `enabled`, the six price → plan/interval mappings, the return
URL, and a `warnings` list (missing price, duplicate price, key that does not
look like `sk_`/`whsec_`, non-absolute return URL). Use Stripe **test mode**
keys first (`sk_test_...`) and card `4242 4242 4242 4242`.

Then confirm the gating from both sides:

```sh
# billing off: must be 501 billing_disabled
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $KEY" https://api.example.eu/v0/billing

# webhook with a bad signature: must be 400 invalid_signature
curl -s -X POST https://api.example.eu/stripe/webhook -H 'Stripe-Signature: t=1,v1=deadbeef' -d '{}'
```

## 5. What Stripe does to an organization

| Stripe event | Effect on `organizations.plan` |
| --- | --- |
| `checkout.session.completed` | reads the subscription, sets the plan of its price |
| `customer.subscription.updated` (`active`, `trialing`) | plan of the price, grace clock cleared |
| `...updated` (`past_due`, `unpaid`) | plan kept, 7-day grace starts |
| `...deleted`, `paused`, `incomplete_expired` | `free` |
| `invoice.payment_failed` | status `past_due` only; the scheduler downgrades after 7 days |

Events are idempotent (`stripe_events.id`), out-of-order events older than the
last applied one are ignored, and every change writes an audit entry
(`billing.update`, actor `stripe`).

## 6. Rolling back

Remove the two secret keys (or set the ConfigMap aside) and restart the pod:
billing returns to `501 billing_disabled`. Existing subscriptions keep running
in Stripe and the organizations keep their current plan; renewals just stop
being reflected. Cancel the subscriptions in the Stripe dashboard, or switch
plans manually with `mai admin org-plan -org <uuid> -plan free`.
