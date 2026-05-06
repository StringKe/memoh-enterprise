package rbac

type (
	PermissionKey    string
	RoleKey          string
	ResourceType     string
	PrincipalType    string
	AssignmentSource string
)

const (
	PermissionSystemLogin PermissionKey = "system.login"
	PermissionSystemAdmin PermissionKey = "system.admin"

	PermissionBotRead              PermissionKey = "bot.read"
	PermissionBotChat              PermissionKey = "bot.chat"
	PermissionBotUpdate            PermissionKey = "bot.update"
	PermissionBotDelete            PermissionKey = "bot.delete"
	PermissionBotPermissionsManage PermissionKey = "bot.permissions.manage"

	PermissionBotGroupRead              PermissionKey = "bot_group.read"
	PermissionBotGroupUse               PermissionKey = "bot_group.use"
	PermissionBotGroupUpdate            PermissionKey = "bot_group.update"
	PermissionBotGroupDelete            PermissionKey = "bot_group.delete"
	PermissionBotGroupPermissionsManage PermissionKey = "bot_group.permissions.manage"
	PermissionBotGroupBotsManage        PermissionKey = "bot_group.bots.manage"
)

const (
	RoleMember      RoleKey = "member"
	RoleAdmin       RoleKey = "admin"
	RoleBotViewer   RoleKey = "bot_viewer"
	RoleBotOperator RoleKey = "bot_operator"
	RoleBotOwner    RoleKey = "bot_owner"

	RoleBotGroupViewer   RoleKey = "bot_group_viewer"
	RoleBotGroupOperator RoleKey = "bot_group_operator"
	RoleBotGroupEditor   RoleKey = "bot_group_editor"
	RoleBotGroupOwner    RoleKey = "bot_group_owner"
)

const (
	ResourceSystem   ResourceType = "system"
	ResourceBot      ResourceType = "bot"
	ResourceBotGroup ResourceType = "bot_group"
)

const (
	PrincipalUser  PrincipalType = "user"
	PrincipalGroup PrincipalType = "group"
)

const (
	SourceSystem AssignmentSource = "system"
	SourceManual AssignmentSource = "manual"
	SourceSSO    AssignmentSource = "sso"
	SourceSCIM   AssignmentSource = "scim"
)

type Check struct {
	UserID        string
	PermissionKey PermissionKey
	ResourceType  ResourceType
	ResourceID    string
}
