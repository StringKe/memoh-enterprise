-- 0081_structured_data

DROP INDEX IF EXISTS idx_structured_data_audit_actor_bot_created_at;
DROP INDEX IF EXISTS idx_structured_data_audit_space_created_at;
DROP TABLE IF EXISTS structured_data_audit;

DROP INDEX IF EXISTS idx_structured_data_grants_space_id;
DROP INDEX IF EXISTS idx_structured_data_grants_space_target_bot_group;
DROP INDEX IF EXISTS idx_structured_data_grants_space_target_bot;
DROP TABLE IF EXISTS structured_data_grants;

DROP INDEX IF EXISTS idx_structured_data_spaces_owner_type;
DROP INDEX IF EXISTS idx_structured_data_spaces_owner_bot_group;
DROP INDEX IF EXISTS idx_structured_data_spaces_owner_bot;
DROP TABLE IF EXISTS structured_data_spaces;
