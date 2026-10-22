#!/bin/sh
# Creates the database roles (owner, runtime, system; none superuser or
# BYPASSRLS) and the extensions that need a superuser, from
# postgres-roles.sql. Runs on first start of an empty volume; idempotent, so
# it can be run again (kubectl exec) to add missing roles or rotate passwords.
set -eu
: "${MAI_OWNER_PASSWORD:?}" "${MAI_APP_PASSWORD:?}" "${MAI_SYSTEM_PASSWORD:?}"
ROLES_SQL=${MAI_ROLES_SQL:-/docker-entrypoint-initdb.d/mai-roles.psql}
psql -v ON_ERROR_STOP=1 -q --username postgres --dbname "${POSTGRES_DB:-mai}" \
  -v owner_pw="$MAI_OWNER_PASSWORD" -v app_pw="$MAI_APP_PASSWORD" -v system_pw="$MAI_SYSTEM_PASSWORD" \
  -v roles_sql="$ROLES_SQL" <<'SQL'
\o /dev/null
select set_config('mai.mai_owner_password', :'owner_pw', false),
       set_config('mai.mai_app_password', :'app_pw', false),
       set_config('mai.mai_system_password', :'system_pw', false);
\o
\i :roles_sql
SQL
