-- DNS verification state for customer domains.
alter table domains
  add column mx_ok           boolean not null default false,
  add column spf_ok          boolean not null default false,
  add column dkim_ok         boolean not null default false,
  add column dmarc_ok        boolean not null default false,
  add column last_checked_at timestamptz;

create index domains_unverified on domains (created_at) where verified_at is null;
