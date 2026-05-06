package runner

import (
	"context"

	"connectrpc.com/connect"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type StructuredDataClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

func NewStructuredDataClient(client runnerv1connect.RunnerSupportServiceClient) *StructuredDataClient {
	return &StructuredDataClient{client: client}
}

func (c *StructuredDataClient) Runtime(lease RunLease) agenttools.StructuredDataRuntime {
	return structuredDataRuntime{client: c, lease: lease}
}

func (c *StructuredDataClient) ListStructuredDataSpaces(ctx context.Context, lease RunLease) ([]agenttools.StructuredDataSpace, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ListStructuredDataSpaces(ctx, connect.NewRequest(&runnerv1.ListStructuredDataSpacesRequest{
		Ref: lease.Ref().Proto(),
	}))
	if err != nil {
		return nil, err
	}
	spaces := make([]agenttools.StructuredDataSpace, 0, len(resp.Msg.GetSpaces()))
	for _, space := range resp.Msg.GetSpaces() {
		spaces = append(spaces, agenttools.StructuredDataSpace{
			ID:              space.GetId(),
			OwnerType:       space.GetOwnerType(),
			OwnerBotID:      space.GetOwnerBotId(),
			OwnerBotGroupID: space.GetOwnerBotGroupId(),
			SchemaName:      space.GetSchemaName(),
			DisplayName:     space.GetDisplayName(),
		})
	}
	return spaces, nil
}

func (c *StructuredDataClient) ExecuteStructuredDataSQL(ctx context.Context, lease RunLease, input agenttools.StructuredDataSQLInput) (agenttools.StructuredDataSQLResult, error) {
	if c == nil || c.client == nil {
		return agenttools.StructuredDataSQLResult{}, ErrSupportClientMissing
	}
	resp, err := c.client.ExecuteStructuredDataSql(ctx, connect.NewRequest(&runnerv1.ExecuteStructuredDataSqlRequest{
		Ref:             lease.Ref().Proto(),
		SpaceId:         input.SpaceID,
		OwnerType:       input.OwnerType,
		OwnerBotId:      input.OwnerBotID,
		OwnerBotGroupId: input.OwnerBotGroupID,
		Sql:             input.SQL,
		MaxRows:         input.MaxRows,
	}))
	if err != nil {
		return agenttools.StructuredDataSQLResult{}, err
	}
	result := resp.Msg.GetResult()
	rows := make([]map[string]any, 0, len(result.GetRows()))
	for _, row := range result.GetRows() {
		rows = append(rows, row.AsMap())
	}
	return agenttools.StructuredDataSQLResult{
		Columns:    append([]string(nil), result.GetColumns()...),
		Rows:       rows,
		RowCount:   result.GetRowCount(),
		CommandTag: result.GetCommandTag(),
		Truncated:  result.GetTruncated(),
		SchemaName: result.GetSchemaName(),
	}, nil
}

type structuredDataRuntime struct {
	client *StructuredDataClient
	lease  RunLease
}

func (r structuredDataRuntime) ListStructuredDataSpaces(ctx context.Context) ([]agenttools.StructuredDataSpace, error) {
	return r.client.ListStructuredDataSpaces(ctx, r.lease)
}

func (r structuredDataRuntime) ExecuteStructuredDataSQL(ctx context.Context, input agenttools.StructuredDataSQLInput) (agenttools.StructuredDataSQLResult, error) {
	return r.client.ExecuteStructuredDataSQL(ctx, r.lease, input)
}
