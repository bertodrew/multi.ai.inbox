-- Sending health: a circuit breaker on hard bounces (doc 08, T1; doc 11).
--
-- send_outcomes records one row per final outcome of an outbound message:
-- sent (accepted by the relay or the recipient MX), rejected (permanent 5xx
-- at send time) and bounced (a delivery status notification received later).
-- complained is reserved for feedback loop reports. The breaker reads the
-- rolling window per inbox and per organization; when the hard bounce rate
-- trips, sending is paused until an operator resumes it.
--
-- inbox_id and message_id carry no foreign key on purpose: deleting an inbox
-- must not wipe the organization's history (and reset its breaker).

create table send_outcomes (
  id         bigserial primary key,
  org_id     uuid not null references organizations(id) on delete cascade,
  inbox_id   uuid not null,
  message_id uuid not null,
  outcome    text not null check (outcome in ('sent', 'rejected', 'bounced', 'complained')),
  created_at timestamptz not null default now(),
  unique (message_id, outcome)
);
create index send_outcomes_inbox on send_outcomes (inbox_id, created_at);
create index send_outcomes_org on send_outcomes (org_id, created_at);

alter table send_outcomes enable row level security;
alter table send_outcomes force row level security;
create policy tenant_isolation on send_outcomes
  using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org());

-- sending_resumed_at starts a fresh window after an operator resumes, so the
-- outcomes that tripped the breaker do not trip it again at once.
alter table inboxes
  add column sending_paused_at     timestamptz,
  add column sending_paused_reason text,
  add column sending_resumed_at    timestamptz;

alter table organizations
  add column sending_paused_at     timestamptz,
  add column sending_paused_reason text,
  add column sending_resumed_at    timestamptz;
