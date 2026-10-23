-- Feedback loop complaints (ARF, RFC 5965) record the complained outcome that
-- migration 0007 reserved. Reports sent to the shared feedback loop address
-- (MAI_FBL_ADDRESS) belong to no organization, so the original outbound
-- message is looked up by its Message-ID across all of them; this index
-- keeps that lookup cheap. No new table: messages already has row level
-- security, and the cross organization lookup runs in the system context.

create index messages_rfc_out on messages (rfc_message_id) where direction = 'out';
