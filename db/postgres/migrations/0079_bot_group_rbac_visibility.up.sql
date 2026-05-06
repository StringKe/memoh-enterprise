-- 0079_bot_group_rbac_visibility
-- Add bot-group scoped RBAC and visibility controls.

ALTER TABLE bot_groups
  ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'private';

ALTER TABLE bot_groups
  DROP CONSTRAINT IF EXISTS bot_groups_visibility_check,
  ADD CONSTRAINT bot_groups_visibility_check CHECK (visibility IN ('private', 'organization', 'public'));

ALTER TABLE iam_roles
  DROP CONSTRAINT IF EXISTS iam_roles_scope_check,
  ADD CONSTRAINT iam_roles_scope_check CHECK (scope IN ('system', 'bot', 'bot_group'));

ALTER TABLE iam_principal_roles
  DROP CONSTRAINT IF EXISTS iam_principal_roles_resource_type_check,
  ADD CONSTRAINT iam_principal_roles_resource_type_check CHECK (resource_type IN ('system', 'bot', 'bot_group'));

ALTER TABLE iam_principal_roles
  DROP CONSTRAINT IF EXISTS iam_principal_roles_resource_id_check,
  ADD CONSTRAINT iam_principal_roles_resource_id_check CHECK (
    (resource_type = 'system' AND resource_id IS NULL)
    OR resource_type IN ('bot', 'bot_group')
  );

INSERT INTO iam_permissions (key, description, is_system)
VALUES
  ('bot_group.read', 'Read bot group data', true),
  ('bot_group.use', 'Use bots in a bot group', true),
  ('bot_group.update', 'Update bot group configuration', true),
  ('bot_group.delete', 'Delete bot group', true),
  ('bot_group.permissions.manage', 'Manage bot group permissions', true),
  ('bot_group.bots.manage', 'Manage bots in a bot group', true)
ON CONFLICT (key) DO NOTHING;

INSERT INTO iam_roles (key, scope, description, is_system)
VALUES
  ('bot_group_viewer', 'bot_group', 'Bot group viewer', true),
  ('bot_group_operator', 'bot_group', 'Bot group operator', true),
  ('bot_group_editor', 'bot_group', 'Bot group editor', true),
  ('bot_group_owner', 'bot_group', 'Bot group owner', true)
ON CONFLICT (key) DO NOTHING;

INSERT INTO iam_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM iam_roles r
JOIN iam_permissions p ON (
  (r.key = 'bot_group_viewer' AND p.key IN ('bot_group.read')) OR
  (r.key = 'bot_group_operator' AND p.key IN ('bot_group.read', 'bot_group.use')) OR
  (r.key = 'bot_group_editor' AND p.key IN ('bot_group.read', 'bot_group.use', 'bot_group.update', 'bot_group.bots.manage')) OR
  (r.key = 'bot_group_owner' AND p.key IN ('bot_group.read', 'bot_group.use', 'bot_group.update', 'bot_group.delete', 'bot_group.permissions.manage', 'bot_group.bots.manage'))
)
ON CONFLICT DO NOTHING;

INSERT INTO iam_principal_roles (principal_type, principal_id, role_id, resource_type, resource_id, source)
SELECT 'user', bg.owner_user_id, r.id, 'bot_group', bg.id, 'system'
FROM bot_groups bg
JOIN iam_roles r ON r.key = 'bot_group_owner'
ON CONFLICT DO NOTHING;
