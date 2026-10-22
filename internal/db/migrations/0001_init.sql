-- multi.ai.inbox core schema. Every tenant table has org_id and row level security.
-- Tenant access: the application sets app.org_id per transaction.
-- System access (SMTP edge, workers): the application sets app.system = 'on'.

create extension if not exists citext;
create extension if not exists pgcrypto;

create table organizations (
  id         uuid primary key default gen_random_uuid(),
  name       text not null check (length(name) between 1 and 200),
  plan       text not null default 'free',
  created_at timestamptz not null default now()
);

create table api_keys (
  id           uuid primary key default gen_random_uuid(),
  org_id       uuid not null references organizations(id) on delete cascade,
  name         text not null default '',
  prefix       text not null,
  hash         bytea not null unique,
  created_at   timestamptz not null default now(),
  last_used_at timestamptz,
  revoked_at   timestamptz
);

-- org_id null means a shared platform domain (free inboxes live there).
create table domains (
  id                uuid primary key default gen_random_uuid(),
  org_id            uuid references organizations(id) on delete cascade,
  name              citext not null unique,
  dkim_selector     text not null default 'mai1',
  dkim_private_key  text,
  verified_at       timestamptz,
  created_at        timestamptz not null default now()
);

create table inboxes (
  id           uuid primary key default gen_random_uuid(),
  org_id       uuid not null references organizations(id) on delete cascade,
  domain_id    uuid not null references domains(id),
  address      citext not null unique,
  display_name text not null default '',
  client_id    text,
  created_at   timestamptz not null default now(),
  updated_at   timestamptz not null default now(),
  unique (org_id, client_id)
);

create table threads (
  id              uuid primary key default gen_random_uuid(),
  org_id          uuid not null references organizations(id) on delete cascade,
  inbox_id        uuid not null references inboxes(id) on delete cascade,
  subject         text not null default '',
  preview         text not null default '',
  labels          text[] not null default '{}',
  message_count   int not null default 0,
  last_message_at timestamptz not null default now(),
  created_at      timestamptz not null default now()
);
create index threads_inbox_recent on threads (inbox_id, last_message_at desc, id desc);
create index threads_org_recent on threads (org_id, last_message_at desc, id desc);

create table messages (
  id               uuid primary key default gen_random_uuid(),
  org_id           uuid not null references organizations(id) on delete cascade,
  inbox_id         uuid not null references inboxes(id) on delete cascade,
  thread_id        uuid not null references threads(id) on delete cascade,
  direction        text not null check (direction in ('in', 'out')),
  rfc_message_id   text not null,
  in_reply_to      text,
  refs             text[] not null default '{}',
  from_addr        text not null,
  to_addrs         text[] not null default '{}',
  cc_addrs         text[] not null default '{}',
  bcc_addrs        text[] not null default '{}',
  subject          text not null default '',
  text_body        text,
  html_body        text,
  preview          text not null default '',
  extracted_text   text,
  labels           text[] not null default '{}',
  raw_key          text,
  size_bytes       int not null default 0,
  status           text not null check (status in ('received', 'queued', 'sending', 'sent', 'failed')),
  auth_results     jsonb not null default '{}',
  gatekeeper       jsonb,
  attempts         int not null default 0,
  next_attempt_at  timestamptz,
  last_error       text,
  created_at       timestamptz not null default now()
);
create index messages_inbox_recent on messages (inbox_id, created_at desc, id desc);
create index messages_thread on messages (thread_id, created_at);
create index messages_rfc on messages (inbox_id, rfc_message_id);
create index messages_outbound_due on messages (next_attempt_at) where status = 'queued';
create index messages_enrich_due on messages (created_at) where gatekeeper is null and direction = 'in';

create table attachments (
  id           uuid primary key default gen_random_uuid(),
  org_id       uuid not null references organizations(id) on delete cascade,
  message_id   uuid not null references messages(id) on delete cascade,
  filename     text not null default '',
  content_type text not null default 'application/octet-stream',
  size_bytes   int not null default 0,
  storage_key  text not null,
  created_at   timestamptz not null default now()
);

create table webhooks (
  id          uuid primary key default gen_random_uuid(),
  org_id      uuid not null references organizations(id) on delete cascade,
  url         text not null,
  secret      text not null,
  event_types text[] not null,
  inbox_ids   uuid[] not null default '{}',
  enabled     boolean not null default true,
  created_at  timestamptz not null default now()
);

create table events (
  id            bigserial primary key,
  event_id      uuid not null unique default gen_random_uuid(),
  org_id        uuid not null references organizations(id) on delete cascade,
  inbox_id      uuid,
  type          text not null,
  payload       jsonb not null,
  created_at    timestamptz not null default now(),
  fanned_out_at timestamptz
);
create index events_pending on events (id) where fanned_out_at is null;
create index events_org on events (org_id, id);

create table webhook_deliveries (
  id               bigserial primary key,
  org_id           uuid not null references organizations(id) on delete cascade,
  webhook_id       uuid not null references webhooks(id) on delete cascade,
  event_id         bigint not null references events(id) on delete cascade,
  attempt          int not null default 0,
  status           text not null default 'pending' check (status in ('pending', 'succeeded', 'failed')),
  next_attempt_at  timestamptz not null default now(),
  last_status_code int,
  last_error       text,
  unique (webhook_id, event_id)
);
create index webhook_deliveries_due on webhook_deliveries (next_attempt_at) where status = 'pending';

create table idempotency_keys (
  org_id     uuid not null references organizations(id) on delete cascade,
  key        text not null,
  request_hash bytea not null,
  response   jsonb not null,
  created_at timestamptz not null default now(),
  primary key (org_id, key)
);

-- Row level security: tenant rows are visible only to their org, or to system context.
create function app_org() returns uuid language sql stable as
$$ select nullif(current_setting('app.org_id', true), '')::uuid $$;
create function app_system() returns boolean language sql stable as
$$ select coalesce(current_setting('app.system', true), '') = 'on' $$;

do $$
declare t text;
begin
  foreach t in array array['api_keys','inboxes','threads','messages','attachments','webhooks',
                           'events','webhook_deliveries','idempotency_keys']
  loop
    execute format('alter table %I enable row level security', t);
    execute format('alter table %I force row level security', t);
    execute format('create policy tenant_isolation on %I using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org())', t);
  end loop;
end $$;

alter table organizations enable row level security;
alter table organizations force row level security;
create policy tenant_isolation on organizations
  using (app_system() or id = app_org()) with check (app_system());

-- Shared domains (org_id null) are readable by every tenant, writable only by system.
alter table domains enable row level security;
alter table domains force row level security;
create policy tenant_read on domains for select using (app_system() or org_id is null or org_id = app_org());
create policy tenant_write on domains for all using (app_system() or org_id = app_org())
  with check (app_system() or org_id = app_org());
