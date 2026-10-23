-- Stripe billing (doc 06 subscriptions, doc 11 section 5.2).
--
-- subscriptions holds one row per organization that started a Stripe
-- Checkout: the Stripe customer and subscription, the Stripe status and the
-- billing period. organizations.plan stays the single source of the limits
-- the code enforces; the webhook handler updates both in one transaction.
--
-- past_due_since starts the grace period: a subscription that is past_due
-- or unpaid keeps its plan for 7 days, then the scheduler moves the
-- organization to free (core.BillingGracePeriod). A successful payment
-- clears it and restores the paid plan.
--
-- last_event_at is the created time of the newest Stripe event applied, so a
-- delayed older event never overwrites newer state.
--
-- Tenants may read their own row (GET /v0/billing); only system context
-- (webhook handler, scheduler) writes, like organizations.

create table subscriptions (
  org_id                 uuid primary key references organizations(id) on delete cascade,
  stripe_customer_id     text unique,
  stripe_subscription_id text unique,
  status                 text not null default 'none'
    check (status in ('none', 'incomplete', 'incomplete_expired', 'trialing', 'active', 'past_due', 'unpaid', 'canceled', 'paused')),
  price_id               text,
  billing_interval       text check (billing_interval in ('monthly', 'yearly')),
  current_period_end     timestamptz,
  cancel_at_period_end   boolean not null default false,
  past_due_since         timestamptz,
  last_event_at          timestamptz,
  created_at             timestamptz not null default now(),
  updated_at             timestamptz not null default now()
);
create index subscriptions_past_due on subscriptions (past_due_since) where past_due_since is not null;

alter table subscriptions enable row level security;
alter table subscriptions force row level security;
create policy tenant_isolation on subscriptions
  using (app_system() or org_id = app_org()) with check (app_system());

-- stripe_events makes webhook processing idempotent: the event id is
-- inserted in the same transaction that applies the event, so a redelivery
-- is a no op and a failed attempt leaves nothing behind for Stripe's retry.
-- System context only; tenants never see it.
create table stripe_events (
  id          text primary key,
  type        text not null,
  org_id      uuid references organizations(id) on delete set null,
  received_at timestamptz not null default now()
);

alter table stripe_events enable row level security;
alter table stripe_events force row level security;
create policy system_only on stripe_events
  using (app_system()) with check (app_system());
