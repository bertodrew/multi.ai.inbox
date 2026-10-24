-- Web push: notifying an installed console about new mail. The device, not the
-- mail, is what is stored here: a subscription holds the endpoint and the two
-- keys the browser generated, and never a message, a sender or a subject.
-- Contract: web/docs/push-design.md
--
-- A subscription belongs to one organization (the tenant of the calling key)
-- and to the person or the API key that registered it. The endpoint is unique
-- per organization: the same browser re-subscribes on every fresh permission
-- grant, and that has to update the row instead of adding one.
--
-- endpoint, p256dh and auth are capabilities: whoever holds them can wake the
-- device, so they are never returned by an API response and never logged. They
-- are stored exactly as the browser sent them, re-encoded to unpadded base64url
-- so a re-subscribe is byte identical.
create table if not exists push_subscriptions (
  id            uuid primary key default gen_random_uuid(),
  org_id        uuid not null references organizations(id) on delete cascade,
  -- Exactly one owner: a person's session or an API key.
  user_id       uuid references users(id) on delete cascade,
  api_key_id    uuid references api_keys(id) on delete cascade,
  endpoint      text not null,
  p256dh        text not null,
  auth          text not null,
  user_agent    text not null default '',
  created_at    timestamptz not null default now(),
  last_used_at  timestamptz,
  failure_count int not null default 0,
  constraint push_subscriptions_endpoint_per_org unique (org_id, endpoint),
  constraint push_subscriptions_owner check (user_id is not null or api_key_id is not null)
);

create index if not exists push_subscriptions_org on push_subscriptions (org_id, created_at desc);
create index if not exists push_subscriptions_key on push_subscriptions (api_key_id) where api_key_id is not null;
create index if not exists push_subscriptions_user on push_subscriptions (user_id) where user_id is not null;

-- Delivery queue. Ingest writes one row per subscription in the same
-- transaction that stores the message, so a push can never fail a delivery and
-- never waits for an HTTP call. The payload is built at enqueue time: it holds
-- the inbox address and a console link, never mail content.
--
-- While a job is pending, one row per subscription and inbox is enough: a burst
-- of mail collapses into a single notification (the partial unique index). A
-- sent job leaves the index, so the next message queues a new one.
create table if not exists push_jobs (
  id              bigserial primary key,
  org_id          uuid not null references organizations(id) on delete cascade,
  subscription_id uuid not null references push_subscriptions(id) on delete cascade,
  inbox_address   text not null,
  payload         jsonb not null,
  attempts        int not null default 0,
  next_attempt_at timestamptz not null default now(),
  sent_at         timestamptz,
  last_error      text not null default '',
  created_at      timestamptz not null default now()
);

create index if not exists push_jobs_ready on push_jobs (next_attempt_at, id) where sent_at is null;
create unique index if not exists push_jobs_pending on push_jobs (subscription_id, inbox_address) where sent_at is null;

-- Tenant isolation, same shape as every other tenant table.
do $$
declare t text;
begin
  foreach t in array array['push_subscriptions', 'push_jobs']
  loop
    execute format('alter table %I enable row level security', t);
    execute format('alter table %I force row level security', t);
    execute format('create policy tenant_isolation on %I using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org())', t);
  end loop;
end $$;
