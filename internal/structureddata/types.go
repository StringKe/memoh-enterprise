package structureddata

import "errors"

type OwnerType string

const (
	OwnerTypeBot      OwnerType = "bot"
	OwnerTypeBotGroup OwnerType = "bot_group"
)

type TargetType string

const (
	TargetTypeBot      TargetType = "bot"
	TargetTypeBotGroup TargetType = "bot_group"
)

type Privilege string

const (
	PrivilegeRead  Privilege = "read"
	PrivilegeWrite Privilege = "write"
	PrivilegeDDL   Privilege = "ddl"
)

var (
	ErrDependencyMissing = errors.New("structured data dependency is not configured")
	ErrInvalidOwner      = errors.New("structured data owner is invalid")
	ErrInvalidTarget     = errors.New("structured data grant target is invalid")
	ErrInvalidPrivilege  = errors.New("structured data privilege is invalid")
	ErrAccessDenied      = errors.New("structured data access denied")
	ErrSQLRequired       = errors.New("structured data sql is required")
)

type OwnerRef struct {
	Type       OwnerType
	BotID      string
	BotGroupID string
}

type GrantInput struct {
	SpaceID          string
	TargetType       TargetType
	TargetBotID      string
	TargetBotGroupID string
	Privileges       []string
	ActorUserID      string
}

type ExecuteInput struct {
	SpaceID         string
	Owner           OwnerRef
	ActorType       string
	ActorUserID     string
	ActorBotID      string
	SQL             string
	MaxRows         int32
	UseOwnerRole    bool
	ExecutionRole   string
	SearchPathSpace string
}

type SQLResult struct {
	Columns    []string
	Rows       []map[string]any
	RowCount   int64
	CommandTag string
	Truncated  bool
	SchemaName string
}

type Table struct {
	SchemaName string
	Name       string
	Columns    []Column
}

type Column struct {
	Name         string
	Type         string
	Nullable     bool
	DefaultValue string
}
