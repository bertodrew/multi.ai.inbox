-- User authentication: people sign in with a magic link, Google or GitHub;
-- agents keep using API keys. Organizations are shared by people, so a role
-- lives on membership, not on the user.
--
-- users.email is the account key: a person who first used GitHub and later a
-- magic link keeps one account, because both prove control of the address.
-- provider and provider_id record the identity that created the account, and
-- provider_id is null for a magic link account (the link itself proves the
-- address). OAuth logins for an existing email attach to that account; a
-- different email creates a different user.
--
-- sessions holds one row per signed in device. The refresh token is stored as
-- a SHA-256 hash, the access token is a short lived signed JWT that is never
-- stored. Rotating a refresh token revokes the old row, so a replay of a used
-- refresh token finds nothing.
--
-- magic_link_tokens and oauth_states are single use and expire in minutes;
-- only their hashes are stored.
--
-- Roles map to organizations through memberships: owner (everything,
-- including deleting the organization), admin (everything except deleting the
-- organization) and member (read plus send, no key or domain management).
-- The permission list each role grants is code, in core.PermissionsForRole,
-- so it stays reviewable next to the API key scopes it mirrors.

create table users (
  id                uuid primary key default gen_random_uuid(),
  email             citext not null unique,
  provider          text not null default 'magic_link' check (provider in ('magic_link', 'google', 'github')),
  provider_id       text,
  name              text not null default '',
  avatar_url        text not null default '',
  email_verified_at timestamptz,
  last_login_at     timestamptz,
  created_at        timestamptz not null default now(),
  updated_at        timestamptz not null default now(),
  constraint users_identity_shape check ((provider = 'magic_link') = (provider_id is null))
);
create unique index users_provider_identity on users (provider, provider_id) where provider_id is not null;

create table memberships (
  org_id     uuid not null references organizations(id) on delete cascade,
  user_id    uuid not null references users(id) on delete cascade,
  role       text not null check (role in ('owner', 'admin', 'member')),
  created_at timestamptz not null default now(),
  primary key (org_id, user_id)
);
create index memberships_user on memberships (user_id);

create table sessions (
  id           uuid primary key default gen_random_uuid(),
  user_id      uuid not null references users(id) on delete cascade,
  org_id       uuid not null references organizations(id) on delete cascade,
  refresh_hash bytea not null unique,
  user_agent   text not null default '',
  ip           text not null default '',
  created_at   timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  expires_at   timestamptz not null,
  revoked_at   timestamptz
);
create index sessions_user on sessions (user_id, created_at desc);
create index sessions_reap on sessions (expires_at) where revoked_at is null;

create table magic_link_tokens (
  id             uuid primary key default gen_random_uuid(),
  token_hash     bytea not null unique,
  email          citext not null,
  redirect_after text not null default '',
  ip             text not null default '',
  created_at     timestamptz not null default now(),
  expires_at     timestamptz not null,
  used_at        timestamptz
);
create index magic_link_email on magic_link_tokens (email, created_at desc);

create table oauth_states (
  state_hash     bytea primary key,
  provider       text not null check (provider in ('google', 'github')),
  code_verifier  text not null,
  redirect_uri   text not null,
  redirect_after text not null default '',
  created_at     timestamptz not null default now(),
  expires_at     timestamptz not null
);

-- People are a platform level record: a tenant role cannot read users,
-- sessions, magic link tokens or OAuth state, not even its own members'
-- credentials. Only system context (the authentication path) touches them.
do $$
declare t text;
begin
  foreach t in array array['users', 'sessions', 'magic_link_tokens', 'oauth_states']
  loop
    execute format('alter table %I enable row level security', t);
    execute format('alter table %I force row level security', t);
    execute format('create policy system_only on %I using (app_system()) with check (app_system())', t);
  end loop;
end $$;

-- Membership is tenant data: a tenant may read who belongs to it. Only system
-- context writes, because a role change is an authorization decision.
alter table memberships enable row level security;
alter table memberships force row level security;
create policy tenant_read on memberships for select using (app_system() or org_id = app_org());
create policy tenant_write on memberships for all using (app_system()) with check (app_system());
