package rbac

import "context"

type PermissionSeed struct {
	Key      PermissionKey
	IsSystem bool
}

type RoleSeed struct {
	Key      RoleKey
	Scope    ResourceType
	IsSystem bool
}

type RolePermissionSeed struct {
	RoleKey       RoleKey
	PermissionKey PermissionKey
}

var BuiltinPermissions = []PermissionSeed{
	{Key: PermissionSystemLogin, IsSystem: true},
	{Key: PermissionSystemAdmin, IsSystem: true},
	{Key: PermissionBotRead, IsSystem: true},
	{Key: PermissionBotChat, IsSystem: true},
	{Key: PermissionBotUpdate, IsSystem: true},
	{Key: PermissionBotDelete, IsSystem: true},
	{Key: PermissionBotPermissionsManage, IsSystem: true},
	{Key: PermissionBotGroupRead, IsSystem: true},
	{Key: PermissionBotGroupUse, IsSystem: true},
	{Key: PermissionBotGroupUpdate, IsSystem: true},
	{Key: PermissionBotGroupDelete, IsSystem: true},
	{Key: PermissionBotGroupPermissionsManage, IsSystem: true},
	{Key: PermissionBotGroupBotsManage, IsSystem: true},
}

var BuiltinRoles = []RoleSeed{
	{Key: RoleMember, Scope: ResourceSystem, IsSystem: true},
	{Key: RoleAdmin, Scope: ResourceSystem, IsSystem: true},
	{Key: RoleBotViewer, Scope: ResourceBot, IsSystem: true},
	{Key: RoleBotOperator, Scope: ResourceBot, IsSystem: true},
	{Key: RoleBotOwner, Scope: ResourceBot, IsSystem: true},
	{Key: RoleBotGroupViewer, Scope: ResourceBotGroup, IsSystem: true},
	{Key: RoleBotGroupOperator, Scope: ResourceBotGroup, IsSystem: true},
	{Key: RoleBotGroupEditor, Scope: ResourceBotGroup, IsSystem: true},
	{Key: RoleBotGroupOwner, Scope: ResourceBotGroup, IsSystem: true},
}

var BuiltinRolePermissions = []RolePermissionSeed{
	{RoleKey: RoleMember, PermissionKey: PermissionSystemLogin},
	{RoleKey: RoleAdmin, PermissionKey: PermissionSystemLogin},
	{RoleKey: RoleAdmin, PermissionKey: PermissionSystemAdmin},
	{RoleKey: RoleBotViewer, PermissionKey: PermissionBotRead},
	{RoleKey: RoleBotViewer, PermissionKey: PermissionBotChat},
	{RoleKey: RoleBotOperator, PermissionKey: PermissionBotRead},
	{RoleKey: RoleBotOperator, PermissionKey: PermissionBotChat},
	{RoleKey: RoleBotOperator, PermissionKey: PermissionBotUpdate},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotRead},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotChat},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotUpdate},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotDelete},
	{RoleKey: RoleBotOwner, PermissionKey: PermissionBotPermissionsManage},
	{RoleKey: RoleBotGroupViewer, PermissionKey: PermissionBotGroupRead},
	{RoleKey: RoleBotGroupOperator, PermissionKey: PermissionBotGroupRead},
	{RoleKey: RoleBotGroupOperator, PermissionKey: PermissionBotGroupUse},
	{RoleKey: RoleBotGroupEditor, PermissionKey: PermissionBotGroupRead},
	{RoleKey: RoleBotGroupEditor, PermissionKey: PermissionBotGroupUse},
	{RoleKey: RoleBotGroupEditor, PermissionKey: PermissionBotGroupUpdate},
	{RoleKey: RoleBotGroupEditor, PermissionKey: PermissionBotGroupBotsManage},
	{RoleKey: RoleBotGroupOwner, PermissionKey: PermissionBotGroupRead},
	{RoleKey: RoleBotGroupOwner, PermissionKey: PermissionBotGroupUse},
	{RoleKey: RoleBotGroupOwner, PermissionKey: PermissionBotGroupUpdate},
	{RoleKey: RoleBotGroupOwner, PermissionKey: PermissionBotGroupDelete},
	{RoleKey: RoleBotGroupOwner, PermissionKey: PermissionBotGroupPermissionsManage},
	{RoleKey: RoleBotGroupOwner, PermissionKey: PermissionBotGroupBotsManage},
}

type BootstrapStore interface {
	EnsurePermissions(ctx context.Context, permissions []PermissionSeed) error
	EnsureRoles(ctx context.Context, roles []RoleSeed) error
	EnsureRolePermissions(ctx context.Context, rolePermissions []RolePermissionSeed) error
}

func Bootstrap(ctx context.Context, store BootstrapStore) error {
	if err := store.EnsurePermissions(ctx, BuiltinPermissions); err != nil {
		return err
	}
	if err := store.EnsureRoles(ctx, BuiltinRoles); err != nil {
		return err
	}
	return store.EnsureRolePermissions(ctx, BuiltinRolePermissions)
}
