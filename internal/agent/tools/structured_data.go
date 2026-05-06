package tools

import (
	"context"
	"errors"

	sdk "github.com/memohai/twilight-ai/sdk"
)

type StructuredDataRuntime interface {
	ListStructuredDataSpaces(ctx context.Context) ([]StructuredDataSpace, error)
	ExecuteStructuredDataSQL(ctx context.Context, input StructuredDataSQLInput) (StructuredDataSQLResult, error)
}

type StructuredDataSpace struct {
	ID              string `json:"id"`
	OwnerType       string `json:"owner_type"`
	OwnerBotID      string `json:"owner_bot_id,omitempty"`
	OwnerBotGroupID string `json:"owner_bot_group_id,omitempty"`
	SchemaName      string `json:"schema_name"`
	DisplayName     string `json:"display_name"`
}

type StructuredDataSQLInput struct {
	SpaceID         string
	OwnerType       string
	OwnerBotID      string
	OwnerBotGroupID string
	SQL             string
	MaxRows         int32
}

type StructuredDataSQLResult struct {
	Columns    []string         `json:"columns"`
	Rows       []map[string]any `json:"rows"`
	RowCount   int64            `json:"row_count"`
	CommandTag string           `json:"command_tag"`
	Truncated  bool             `json:"truncated"`
	SchemaName string           `json:"schema_name"`
}

type StructuredDataProvider struct {
	runtime StructuredDataRuntime
}

func NewStructuredDataProvider(runtime StructuredDataRuntime) *StructuredDataProvider {
	return &StructuredDataProvider{runtime: runtime}
}

func (p *StructuredDataProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if p == nil || p.runtime == nil || session.IsSubagent {
		return nil, nil
	}
	return []sdk.Tool{
		{
			Name:        "structured_data_spaces",
			Description: "List PostgreSQL structured data spaces available to this bot. Use this before running SQL against shared bot or bot-group data.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx *sdk.ToolExecContext, _ any) (any, error) {
				spaces, err := p.runtime.ListStructuredDataSpaces(ctx.Context)
				if err != nil {
					return nil, err
				}
				return map[string]any{"spaces": spaces}, nil
			},
		},
		{
			Name:        "structured_data_sql",
			Description: "Run raw SQL, including DDL, in a bot or bot-group PostgreSQL structured data space. PostgreSQL role and schema grants enforce access.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"space_id":           map[string]any{"type": "string"},
					"owner_type":         map[string]any{"type": "string", "enum": []string{"bot", "bot_group"}},
					"owner_bot_id":       map[string]any{"type": "string"},
					"owner_bot_group_id": map[string]any{"type": "string"},
					"sql":                map[string]any{"type": "string"},
					"max_rows":           map[string]any{"type": "integer", "minimum": 1, "maximum": 5000},
				},
				"required": []string{"sql"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				maxRows, _, err := IntArg(args, "max_rows")
				if err != nil {
					return nil, err
				}
				if maxRows < 0 || maxRows > 5000 {
					return nil, errors.New("max_rows must be between 1 and 5000")
				}
				maxRows32 := int32(maxRows)
				sql := StringArg(args, "sql")
				if sql == "" {
					return nil, errors.New("sql is required")
				}
				result, err := p.runtime.ExecuteStructuredDataSQL(ctx.Context, StructuredDataSQLInput{
					SpaceID:         StringArg(args, "space_id"),
					OwnerType:       StringArg(args, "owner_type"),
					OwnerBotID:      StringArg(args, "owner_bot_id"),
					OwnerBotGroupID: StringArg(args, "owner_bot_group_id"),
					SQL:             sql,
					MaxRows:         maxRows32,
				})
				if err != nil {
					return nil, err
				}
				return result, nil
			},
		},
	}, nil
}
