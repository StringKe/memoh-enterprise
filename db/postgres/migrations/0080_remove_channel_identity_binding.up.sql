-- 0080_remove_channel_identity_binding
-- Remove one-time bind codes for channel identities; enterprise fork keeps
-- the channel_identities.user_id column for SSO/IAM linkage.

DROP TABLE IF EXISTS channel_identity_bind_codes;
