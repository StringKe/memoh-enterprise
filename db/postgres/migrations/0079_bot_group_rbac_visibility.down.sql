-- 0079_bot_group_rbac_visibility
-- Remove bot-group scoped RBAC and visibility controls.

DELETE FROM iam_principal_roles pr
USING iam_roles r
WHERE pr.role_id = r.id
  AND r.key IN ('bot_group_viewer', 'bot_group_operator', 'bot_group_editor', 'bot_group_owner');

DELETE FROM iam_role_permissions rp
USING iam_roles r
WHERE rp.role_id = r.id
  AND r.key IN ('bot_group_viewer', 'bot_group_operator', 'bot_group_editor', 'bot_group_owner');

DELETE FROM iam_roles
WHERE key IN ('bot_group_viewer', 'bot_group_operator', 'bot_group_editor', 'bot_group_owner');

DELETE FROM iam_permissions
WHERE key IN (
  'bot_group.read',
  'bot_group.use',
  'bot_group.update',
  'bot_group.delete',
  'bot_group.permissions.manage',
  'bot_group.bots.manage'
);

ALTER TABLE iam_principal_roles
  DROP CONSTRAINT IF EXISTS iam_principal_roles_resource_type_check,
  ADD CONSTRAINT iam_principal_roles_resource_type_check CHECK (resource_type IN ('system', 'bot'));

ALTER TABLE iam_principal_roles
  DROP CONSTRAINT IF EXISTS iam_principal_roles_resource_id_check,
  ADD CONSTRAINT iam_principal_roles_resource_id_check CHECK (
    (resource_type = 'system' AND resource_id IS NULL)
    OR resource_type = 'bot'
  );

ALTER TABLE iam_roles
  DROP CONSTRAINT IF EXISTS iam_roles_scope_check,
  ADD CONSTRAINT iam_roles_scope_check CHECK (scope IN ('system', 'bot'));

ALTER TABLE bot_groups
  DROP CONSTRAINT IF EXISTS bot_groups_visibility_check,
  DROP COLUMN IF EXISTS visibility;
