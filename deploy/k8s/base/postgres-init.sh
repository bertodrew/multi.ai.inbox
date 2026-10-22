#!/bin/sh
# Creates the application role (no superuser, no BYPASSRLS) and the extensions
# that need a superuser. Runs once, on first start of an empty volume.
set -eu
psql -v ON_ERROR_STOP=1 --username postgres --dbname mai <<SQL
create role mai_app login password '${MAI_APP_PASSWORD}' nosuperuser nobypassrls;
alter database mai owner to mai_app;
alter schema public owner to mai_app;
create extension if not exists citext;
create extension if not exists pgcrypto;
SQL
