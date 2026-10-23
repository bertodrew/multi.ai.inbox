-- Tickets: the human side of an inbox. A ticket is a thread with state; the
-- message body is never copied. Design: docs/agentmail-re/15-ticketing.md
-- Review conditions: docs/agentmail-re/16-ticketing-review.md

-- An inbox can be marked as a support intake: its new threads become tickets.
alter table inboxes add column if not exists ticket_intake boolean not null default false;

create table if not exists tickets (
  id               uuid primary key default gen_random_uuid(),
  org_id           uuid not null references organizations(id) on delete cascade,
  number           bigint not null,
  inbox_id         uuid not null references inboxes(id) on delete cascade,
  thread_id        uuid not null references threads(id) on delete cascade,
  subject          text not null default '',
  requester        text not null default '',
  state            text not null default 'new'
                   check (state in ('new','open','pending','resolved','closed')),
  priority         text not null default 'normal'
                   check (priority in ('low','normal','high','urgent')),
  assignee_user    uuid references users(id) on delete set null,
  assignee_key     uuid references api_keys(id) on delete set null,
  sla_policy       text,
  due_first_reply  timestamptz,
  due_resolution   timestamptz,
  first_reply_at   timestamptz,
  resolved_at      timestamptz,
  closed_at        timestamptz,
  tags             text[] not null default '{}',
  created_at       timestamptz not null default now(),
  updated_at       timestamptz not null default now(),
  unique (org_id, number),
  unique (thread_id)
);

create index if not exists tickets_queue on tickets (org_id, state, updated_at desc);
create index if not exists tickets_assignee on tickets (org_id, assignee_user, state);
create index if not exists tickets_sla on tickets (org_id, state, due_first_reply)
  where state in ('new','open');

-- Internal notes are their own table on purpose: an internal note must be
-- impossible to email by accident. There is no code path from here to the
-- sender, and a test proves it.
create table if not exists ticket_notes (
  id          uuid primary key default gen_random_uuid(),
  org_id      uuid not null references organizations(id) on delete cascade,
  ticket_id   uuid not null references tickets(id) on delete cascade,
  author_user uuid references users(id) on delete set null,
  author_key  uuid references api_keys(id) on delete set null,
  body        text not null,
  created_at  timestamptz not null default now()
);

create index if not exists ticket_notes_ticket on ticket_notes (ticket_id, created_at);

-- Timeline and audit trail: every state, priority, assignment, note, reply and
-- SLA breach, with the human or the key that did it.
create table if not exists ticket_events (
  id         bigserial primary key,
  org_id     uuid not null references organizations(id) on delete cascade,
  ticket_id  uuid not null references tickets(id) on delete cascade,
  kind       text not null,
  actor_user uuid references users(id) on delete set null,
  actor_key  uuid references api_keys(id) on delete set null,
  detail     jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create index if not exists ticket_events_ticket on ticket_events (ticket_id, created_at);

-- Tenant isolation, same shape as every other tenant table.
do $$
declare t text;
begin
  foreach t in array array['tickets', 'ticket_notes', 'ticket_events']
  loop
    execute format('alter table %I enable row level security', t);
    execute format('alter table %I force row level security', t);
    execute format('create policy tenant_isolation on %I using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org())', t);
  end loop;
end $$;
