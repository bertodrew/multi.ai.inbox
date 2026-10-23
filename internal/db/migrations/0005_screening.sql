-- Screen before notify (doc 12, risk R08, OWASP LLM01).
--
-- organizations.screen_before_notify: when on, inbound mail is held until the
-- gatekeeper verdict exists. message.received is emitted only for a clean
-- verdict, or after a privileged release of a message held for review.
-- Default on, also for existing organizations.
--
-- messages.screening is the hold state of an inbound message:
--   null      not held (outbound mail, or the organization had the option off at ingest)
--   pending   waiting for the gatekeeper; never listed, never notified
--   review    verdict review, gatekeeper unavailable or screening timed out;
--             hidden unless the caller asks for include_review and may see it
--   allowed   clean verdict; visible and notified
--   released  a privileged key released it from review; visible and notified
--
-- No new tables: both columns live on tables that already have row level
-- security (organizations: tenants read their own row, only system context
-- writes; messages: tenant isolation), so the existing policies apply.

alter table organizations
  add column screen_before_notify boolean not null default true;

alter table messages
  add column screening text check (screening in ('pending', 'review', 'allowed', 'released'));

-- Held messages are looked up by the timeout sweep and the review listings.
create index messages_held on messages (created_at) where screening in ('pending', 'review');
