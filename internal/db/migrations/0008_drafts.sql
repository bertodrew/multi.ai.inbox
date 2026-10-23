-- Drafts (doc 05, Draft; user story E3.3): a message an agent prepares and
-- sends later, by hand (POST .../drafts/{draft}/send) or at send_at through
-- the scheduler. Sending goes through the normal send path (plan limits,
-- sending pause, footer) and deletes the draft in the same transaction.
--
-- A reply draft stores the threading it was created with (thread_id,
-- in_reply_to, refs), so it keeps its thread even if it is sent days later.
-- Attachments are kept inline as the request gave them (base64, at most
-- 6 MB in total, as for a send) and never reach the blob store, so deleting a
-- draft or its inbox leaves nothing behind.
--
-- status: draft (no send_at), scheduled (the scheduler sends it at send_at),
-- failed (a scheduled send was refused; last_error says why, a patch or a
-- manual send retries).

create table drafts (
  id          uuid primary key default gen_random_uuid(),
  org_id      uuid not null references organizations(id) on delete cascade,
  inbox_id    uuid not null references inboxes(id) on delete cascade,
  thread_id   uuid references threads(id) on delete cascade,
  in_reply_to text,
  refs        text[] not null default '{}',
  to_addrs    text[] not null default '{}',
  cc_addrs    text[] not null default '{}',
  bcc_addrs   text[] not null default '{}',
  subject     text not null default '',
  text_body   text not null default '',
  html_body   text not null default '',
  labels      text[] not null default '{}',
  attachments jsonb not null default '[]',
  send_at     timestamptz,
  status      text not null default 'draft' check (status in ('draft', 'scheduled', 'failed')),
  last_error  text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now(),
  check ((status = 'scheduled') = (send_at is not null) or status = 'failed')
);
create index drafts_inbox_recent on drafts (inbox_id, created_at desc, id desc);
create index drafts_due on drafts (send_at) where status = 'scheduled';

alter table drafts enable row level security;
alter table drafts force row level security;
create policy tenant_isolation on drafts
  using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org());
