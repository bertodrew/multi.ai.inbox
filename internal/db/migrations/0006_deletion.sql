-- Durable blob deletion and the 90 day address quarantine (doc 08, T6).
--
-- Deleting rows never touches the blob store directly. Triggers queue every
-- storage key of a deleted message or attachment in blob_deletions, in the
-- same transaction as the delete, so a rollback queues nothing and a crash
-- after commit leaves the keys queued. The scheduler role removes the blobs
-- and then the queue rows; removing a missing blob counts as done, so a
-- retry is harmless. The triggers fire for every path that deletes rows,
-- including the cascade from an organization delete.

create table blob_deletions (
  id              bigserial primary key,
  org_id          uuid not null,            -- no foreign key: the org may be gone already
  storage_key     text not null,
  attempts        int not null default 0,
  next_attempt_at timestamptz not null default now(),
  last_error      text,
  created_at      timestamptz not null default now()
);
create index blob_deletions_due on blob_deletions (next_attempt_at);

-- A deleted address cannot be created again by any organization until
-- quarantined_until, so nobody receives mail (password resets, invoices)
-- meant for the previous owner. Only the system context (operator support
-- tooling) may shorten or lift a quarantine; expired rows are purged by the
-- scheduler.
create table address_quarantine (
  id                bigserial primary key,
  org_id            uuid not null,          -- previous owner, kept for support
  address           citext not null,
  deleted_at        timestamptz not null default now(),
  quarantined_until timestamptz not null
);
create index address_quarantine_address on address_quarantine (address, quarantined_until desc);

alter table blob_deletions enable row level security;
alter table blob_deletions force row level security;
create policy tenant_isolation on blob_deletions
  using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org());

-- Every tenant must see quarantined addresses (creating an inbox checks them
-- across organizations); only the deleting org, or the system, writes rows.
alter table address_quarantine enable row level security;
alter table address_quarantine force row level security;
create policy tenant_read on address_quarantine for select using (true);
create policy tenant_insert on address_quarantine for insert with check (app_system() or org_id = app_org());
create policy system_update on address_quarantine for update using (app_system()) with check (app_system());
create policy system_delete on address_quarantine for delete using (app_system());

create function queue_message_blob() returns trigger language plpgsql as $$
begin
  if old.raw_key is not null and old.raw_key <> '' then
    insert into blob_deletions (org_id, storage_key) values (old.org_id, old.raw_key);
  end if;
  return old;
end $$;

create function queue_attachment_blob() returns trigger language plpgsql as $$
begin
  insert into blob_deletions (org_id, storage_key) values (old.org_id, old.storage_key);
  return old;
end $$;

create function quarantine_inbox_address() returns trigger language plpgsql as $$
begin
  insert into address_quarantine (org_id, address, quarantined_until)
    values (old.org_id, old.address, now() + interval '90 days');
  return old;
end $$;

create trigger messages_queue_blob after delete on messages
  for each row execute function queue_message_blob();
create trigger attachments_queue_blob after delete on attachments
  for each row execute function queue_attachment_blob();
create trigger inboxes_quarantine after delete on inboxes
  for each row execute function quarantine_inbox_address();
