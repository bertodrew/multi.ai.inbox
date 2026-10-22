-- Role split (risk R11 in docs/agentmail-re/12). Runs as the owner role
-- mai_owner in production (deploy/k8s/base/postgres-roles.sql creates the
-- roles with passwords); in development it may run as a superuser, and the
-- roles are then created here, without login, if they are missing.
--
-- mai_app (tenant requests) and mai_system (SMTP edge, workers,
-- authentication) get DML only, never ownership, so neither can disable row
-- level security, drop policies or alter tables. The system bypass now needs
-- both app.system = 'on' and membership of mai_system: a mai_app session that
-- sets app.system gains nothing.

do $$
declare
  r text;
begin
  foreach r in array array['mai_owner', 'mai_app', 'mai_system'] loop
    if not exists (select from pg_roles where rolname = r) then
      begin
        execute format('create role %I nologin nosuperuser nobypassrls', r);
      exception
        when duplicate_object then null;
        when insufficient_privilege then
          raise exception 'role % is missing: run deploy/k8s/base/postgres-roles.sql as a superuser first', r;
      end;
    end if;
  end loop;
  if pg_has_role('mai_app', 'mai_system', 'member') then
    raise exception 'mai_app must not be a member of mai_system';
  end if;
  -- Data migrations run by the owner under forced RLS need the system context.
  if not pg_has_role('mai_owner', 'mai_system', 'member') then
    begin
      grant mai_system to mai_owner;
    exception when insufficient_privilege then
      raise notice 'could not grant mai_system to mai_owner; run deploy/k8s/base/postgres-roles.sql as a superuser';
    end;
  end if;
end $$;

-- Honour app.system only for members of mai_system (superusers bypass RLS
-- anyway). Security invoker on purpose: current_user is the session role.
create or replace function app_system() returns boolean language sql stable as
$$ select coalesce(current_setting('app.system', true), '') = 'on'
          and pg_has_role(current_user, 'mai_system', 'member') $$;

grant usage on schema public to mai_app, mai_system;
grant select, insert, update, delete on all tables in schema public to mai_app, mai_system;
grant usage, select on all sequences in schema public to mai_app, mai_system;
-- The runtime may check which migrations are applied, never change them.
revoke all on schema_migrations from mai_app, mai_system;
grant select on schema_migrations to mai_app, mai_system;

-- Future migrations: tables and sequences created by the role running them
-- (and by mai_owner, when a superuser runs them) get the same grants.
alter default privileges in schema public
  grant select, insert, update, delete on tables to mai_app, mai_system;
alter default privileges in schema public
  grant usage, select on sequences to mai_app, mai_system;
do $$
begin
  if current_user <> 'mai_owner' and pg_has_role(current_user, 'mai_owner', 'member') then
    alter default privileges for role mai_owner in schema public
      grant select, insert, update, delete on tables to mai_app, mai_system;
    alter default privileges for role mai_owner in schema public
      grant usage, select on sequences to mai_app, mai_system;
  end if;
end $$;
