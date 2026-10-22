-- Scoped API keys and an audit trail of security relevant actions.

-- permissions null means every permission (keys created before scopes existed,
-- and the operator's admin keys).
alter table api_keys
  add column permissions text[],
  add column expires_at  timestamptz,
  add column created_by  uuid references api_keys(id) on delete set null;

create table audit_log (
  id          bigserial primary key,
  org_id      uuid not null references organizations(id) on delete cascade,
  actor       text not null,            -- api_key:<id>, mcp:<id>, operator
  action      text not null,            -- e.g. api_key.create, inbox.delete
  target      text not null default '',
  ip          text not null default '',
  request_id  text not null default '',
  metadata    jsonb not null default '{}',
  created_at  timestamptz not null default now()
);
create index audit_log_org on audit_log (org_id, id desc);

alter table audit_log enable row level security;
alter table audit_log force row level security;
create policy tenant_isolation on audit_log
  using (app_system() or org_id = app_org()) with check (app_system() or org_id = app_org());
