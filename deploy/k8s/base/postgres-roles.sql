-- Database roles for multi.ai.inbox. Run as a superuser, connected to the
-- application database. Idempotent: safe to run again to rotate passwords.
--
--   mai_owner   owns the schema and every table; used only by `mai migrate`
--   mai_app     runtime role for tenant requests: DML only, never owner
--   mai_system  runtime role for the SMTP edge, workers and authentication:
--               DML only; the only role whose app.system setting is honoured
--
-- None of them is superuser or has BYPASSRLS, so row level security always
-- applies. Grants on tables come from migration 0004_roles.sql (default
-- privileges), because tables are created by migrations, not here.
--
-- Passwords are read from the session settings mai.mai_owner_password,
-- mai.mai_app_password and mai.mai_system_password (set them with set_config
-- before running this file); an empty or missing setting leaves the password
-- unchanged. Used by postgres-init.sh and by the end to end test.

do $$
declare
  r  text;
  pw text;
begin
  foreach r in array array['mai_owner', 'mai_app', 'mai_system'] loop
    begin
      execute format('create role %I', r);
    exception when duplicate_object then
      null;
    end;
    execute format('alter role %I login nosuperuser nocreatedb nocreaterole noreplication nobypassrls', r);
    pw := nullif(current_setting('mai.' || r || '_password', true), '');
    if pw is not null then
      execute format('alter role %I password %L', r, pw);
    end if;
  end loop;
  -- The owner runs data migrations under forced RLS, so it may use the
  -- system context. The runtime role mai_app must never be a member.
  if not pg_has_role('mai_owner', 'mai_system', 'member') then
    grant mai_system to mai_owner;
  end if;
  if pg_has_role('mai_app', 'mai_system', 'member') then
    revoke mai_system from mai_app;
  end if;
  execute format('alter database %I owner to mai_owner', current_database());
  execute format('revoke all on database %I from public', current_database());
  execute format('grant connect, temporary on database %I to mai_owner, mai_app, mai_system', current_database());
end $$;

alter schema public owner to mai_owner;
revoke all on schema public from public;
grant usage on schema public to mai_app, mai_system;

-- Extensions need a superuser.
create extension if not exists citext;
create extension if not exists pgcrypto;
