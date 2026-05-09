-- 0084_remove_channel_identity_binding
-- 上游 cherry-pick：移除 one-time bind codes 表。
-- enterprise fork 在 0077_iam_sso_rbac 已把 channel_identity_bind_codes 重命名为
-- iam_channel_identity_bind_codes 并归 IAM 模块所有，因此本 migration 在
-- enterprise 上为 no-op（保留迁移序号以避免再次回收冲突）。

SELECT 1;
