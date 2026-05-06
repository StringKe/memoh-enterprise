package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/memohai/memoh/internal/botgroups"
	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/iam/rbac"
	"github.com/memohai/memoh/internal/structureddata"
)

type StructuredDataService struct {
	data   *structureddata.Service
	bots   structuredDataBotPermissionChecker
	groups structuredDataGroupPermissionChecker
}

type structuredDataBotPermissionChecker interface {
	HasBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) (bool, error)
}

type structuredDataGroupPermissionChecker interface {
	HasGroupPermission(ctx context.Context, userID, groupID string, permission rbac.PermissionKey) (bool, error)
}

func NewStructuredDataService(data *structureddata.Service, bots *bots.Service, groups *botgroups.Service) *StructuredDataService {
	return &StructuredDataService{data: data, bots: bots, groups: groups}
}

func NewStructuredDataHandler(service *StructuredDataService) Handler {
	path, handler := privatev1connect.NewStructuredDataServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *StructuredDataService) EnsureStructuredDataSpace(ctx context.Context, req *connect.Request[privatev1.EnsureStructuredDataSpaceRequest]) (*connect.Response[privatev1.EnsureStructuredDataSpaceResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	owner := structuredDataOwnerFromRequest(req.Msg.GetOwnerType(), req.Msg.GetOwnerBotId(), req.Msg.GetOwnerBotGroupId())
	if err := s.requireOwnerPermission(ctx, userID, owner, rbac.PermissionBotUpdate, rbac.PermissionBotGroupUpdate); err != nil {
		return nil, structuredDataConnectError(err)
	}
	space, err := s.data.EnsureSpace(ctx, owner)
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	return connect.NewResponse(&privatev1.EnsureStructuredDataSpaceResponse{Space: structuredDataSpaceToProto(space)}), nil
}

func (s *StructuredDataService) ListStructuredDataSpaces(ctx context.Context, req *connect.Request[privatev1.ListStructuredDataSpacesRequest]) (*connect.Response[privatev1.ListStructuredDataSpacesResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	owner := structuredDataOwnerFromRequest(req.Msg.GetOwnerType(), req.Msg.GetOwnerBotId(), req.Msg.GetOwnerBotGroupId())
	if owner.Type != "" {
		if err := s.requireOwnerPermission(ctx, userID, owner, rbac.PermissionBotRead, rbac.PermissionBotGroupRead); err != nil {
			return nil, structuredDataConnectError(err)
		}
		space, err := s.data.EnsureSpace(ctx, owner)
		if err != nil {
			return nil, structuredDataConnectError(err)
		}
		return connect.NewResponse(&privatev1.ListStructuredDataSpacesResponse{
			Spaces: []*privatev1.StructuredDataSpace{structuredDataSpaceToProto(space)},
			Page:   &privatev1.PageResponse{},
		}), nil
	}
	spaces, err := s.data.ListSpaces(ctx)
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	out := make([]*privatev1.StructuredDataSpace, 0, len(spaces))
	for _, space := range spaces {
		allowed, err := s.canAccessSpace(ctx, userID, space, rbac.PermissionBotRead, rbac.PermissionBotGroupRead)
		if err != nil {
			return nil, structuredDataConnectError(err)
		}
		if allowed {
			out = append(out, structuredDataSpaceToProto(space))
		}
	}
	return connect.NewResponse(&privatev1.ListStructuredDataSpacesResponse{Spaces: out, Page: &privatev1.PageResponse{}}), nil
}

func (s *StructuredDataService) DescribeStructuredDataSpace(ctx context.Context, req *connect.Request[privatev1.DescribeStructuredDataSpaceRequest]) (*connect.Response[privatev1.DescribeStructuredDataSpaceResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	space, tables, err := s.data.DescribeSpace(ctx, req.Msg.GetSpaceId())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	if err := s.requireSpacePermission(ctx, userID, space, rbac.PermissionBotRead, rbac.PermissionBotGroupRead); err != nil {
		return nil, structuredDataConnectError(err)
	}
	return connect.NewResponse(&privatev1.DescribeStructuredDataSpaceResponse{
		Space:  structuredDataSpaceToProto(space),
		Tables: structuredDataTablesToProto(tables),
	}), nil
}

func (s *StructuredDataService) ExecuteStructuredDataSql(ctx context.Context, req *connect.Request[privatev1.ExecuteStructuredDataSqlRequest]) (*connect.Response[privatev1.ExecuteStructuredDataSqlResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	space, err := s.data.GetSpace(ctx, req.Msg.GetSpaceId())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	if err := s.requireSpacePermission(ctx, userID, space, rbac.PermissionBotUpdate, rbac.PermissionBotGroupUpdate); err != nil {
		return nil, structuredDataConnectError(err)
	}
	result, err := s.data.ExecuteAsOwner(ctx, structureddata.ExecuteInput{
		SpaceID:     req.Msg.GetSpaceId(),
		ActorUserID: userID,
		SQL:         req.Msg.GetSql(),
		MaxRows:     req.Msg.GetMaxRows(),
	})
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	return connect.NewResponse(&privatev1.ExecuteStructuredDataSqlResponse{Result: structuredDataSQLResultToProto(result)}), nil
}

func (s *StructuredDataService) ListStructuredDataGrants(ctx context.Context, req *connect.Request[privatev1.ListStructuredDataGrantsRequest]) (*connect.Response[privatev1.ListStructuredDataGrantsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	space, err := s.data.GetSpace(ctx, req.Msg.GetSpaceId())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	if err := s.requireSpacePermission(ctx, userID, space, rbac.PermissionBotPermissionsManage, rbac.PermissionBotGroupPermissionsManage); err != nil {
		return nil, structuredDataConnectError(err)
	}
	grants, err := s.data.ListGrants(ctx, req.Msg.GetSpaceId())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	out := make([]*privatev1.StructuredDataGrant, 0, len(grants))
	for _, grant := range grants {
		out = append(out, structuredDataGrantToProto(grant))
	}
	return connect.NewResponse(&privatev1.ListStructuredDataGrantsResponse{Grants: out, Page: &privatev1.PageResponse{}}), nil
}

func (s *StructuredDataService) UpsertStructuredDataGrant(ctx context.Context, req *connect.Request[privatev1.UpsertStructuredDataGrantRequest]) (*connect.Response[privatev1.UpsertStructuredDataGrantResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	space, err := s.data.GetSpace(ctx, req.Msg.GetSpaceId())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	if err := s.requireSpacePermission(ctx, userID, space, rbac.PermissionBotPermissionsManage, rbac.PermissionBotGroupPermissionsManage); err != nil {
		return nil, structuredDataConnectError(err)
	}
	grant, err := s.data.UpsertGrant(ctx, structureddata.GrantInput{
		SpaceID:          req.Msg.GetSpaceId(),
		TargetType:       structureddata.TargetType(req.Msg.GetTargetType()),
		TargetBotID:      req.Msg.GetTargetBotId(),
		TargetBotGroupID: req.Msg.GetTargetBotGroupId(),
		Privileges:       req.Msg.GetPrivileges(),
		ActorUserID:      userID,
	})
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpsertStructuredDataGrantResponse{Grant: structuredDataGrantToProto(grant)}), nil
}

func (s *StructuredDataService) DeleteStructuredDataGrant(ctx context.Context, req *connect.Request[privatev1.DeleteStructuredDataGrantRequest]) (*connect.Response[privatev1.DeleteStructuredDataGrantResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	grant, err := s.data.GetGrant(ctx, req.Msg.GetGrantId())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	space, err := s.data.GetSpace(ctx, grant.SpaceID.String())
	if err != nil {
		return nil, structuredDataConnectError(err)
	}
	if err := s.requireSpacePermission(ctx, userID, space, rbac.PermissionBotPermissionsManage, rbac.PermissionBotGroupPermissionsManage); err != nil {
		return nil, structuredDataConnectError(err)
	}
	if err := s.data.DeleteGrant(ctx, req.Msg.GetGrantId(), userID); err != nil {
		return nil, structuredDataConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteStructuredDataGrantResponse{}), nil
}

func (s *StructuredDataService) requireOwnerPermission(ctx context.Context, userID string, owner structureddata.OwnerRef, botPermission rbac.PermissionKey, groupPermission rbac.PermissionKey) error {
	switch owner.Type {
	case structureddata.OwnerTypeBot:
		return s.requireBotPermission(ctx, userID, owner.BotID, botPermission)
	case structureddata.OwnerTypeBotGroup:
		return s.requireGroupPermission(ctx, userID, owner.BotGroupID, groupPermission)
	default:
		return structureddata.ErrInvalidOwner
	}
}

func (s *StructuredDataService) requireSpacePermission(ctx context.Context, userID string, space dbsqlc.StructuredDataSpace, botPermission rbac.PermissionKey, groupPermission rbac.PermissionKey) error {
	allowed, err := s.canAccessSpace(ctx, userID, space, botPermission, groupPermission)
	if err != nil {
		return err
	}
	if !allowed {
		return structureddata.ErrAccessDenied
	}
	return nil
}

func (s *StructuredDataService) canAccessSpace(ctx context.Context, userID string, space dbsqlc.StructuredDataSpace, botPermission rbac.PermissionKey, groupPermission rbac.PermissionKey) (bool, error) {
	switch space.OwnerType {
	case string(structureddata.OwnerTypeBot):
		if !space.OwnerBotID.Valid {
			return false, structureddata.ErrInvalidOwner
		}
		return s.canBot(ctx, userID, space.OwnerBotID.String(), botPermission)
	case string(structureddata.OwnerTypeBotGroup):
		if !space.OwnerBotGroupID.Valid {
			return false, structureddata.ErrInvalidOwner
		}
		return s.canGroup(ctx, userID, space.OwnerBotGroupID.String(), groupPermission)
	default:
		return false, structureddata.ErrInvalidOwner
	}
}

func (s *StructuredDataService) requireBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) error {
	allowed, err := s.canBot(ctx, userID, botID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return bots.ErrBotAccessDenied
	}
	return nil
}

func (s *StructuredDataService) canBot(ctx context.Context, userID, botID string, permission rbac.PermissionKey) (bool, error) {
	if s.bots == nil {
		return false, structureddata.ErrDependencyMissing
	}
	return s.bots.HasBotPermission(ctx, userID, botID, permission)
}

func (s *StructuredDataService) requireGroupPermission(ctx context.Context, userID, groupID string, permission rbac.PermissionKey) error {
	allowed, err := s.canGroup(ctx, userID, groupID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return botgroups.ErrGroupAccessDenied
	}
	return nil
}

func (s *StructuredDataService) canGroup(ctx context.Context, userID, groupID string, permission rbac.PermissionKey) (bool, error) {
	if s.groups == nil {
		return false, structureddata.ErrDependencyMissing
	}
	return s.groups.HasGroupPermission(ctx, userID, groupID, permission)
}

func structuredDataOwnerFromRequest(ownerType string, botID string, botGroupID string) structureddata.OwnerRef {
	return structureddata.OwnerRef{
		Type:       structureddata.OwnerType(ownerType),
		BotID:      botID,
		BotGroupID: botGroupID,
	}
}

func structuredDataConnectError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, structureddata.ErrAccessDenied):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, structureddata.ErrDependencyMissing):
		return connect.NewError(connect.CodeInternal, err)
	case errors.Is(err, structureddata.ErrInvalidOwner), errors.Is(err, structureddata.ErrInvalidTarget), errors.Is(err, structureddata.ErrInvalidPrivilege), errors.Is(err, structureddata.ErrSQLRequired):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connectError(err)
	}
}

func structuredDataSpaceToProto(row dbsqlc.StructuredDataSpace) *privatev1.StructuredDataSpace {
	return &privatev1.StructuredDataSpace{
		Id:              row.ID.String(),
		OwnerType:       row.OwnerType,
		OwnerBotId:      uuidString(row.OwnerBotID),
		OwnerBotGroupId: uuidString(row.OwnerBotGroupID),
		SchemaName:      row.SchemaName,
		RoleName:        row.RoleName,
		DisplayName:     row.DisplayName,
		Metadata:        mapToStruct(jsonObject(row.Metadata)),
		Audit:           &privatev1.AuditFields{CreatedAt: pgTimeToProto(row.CreatedAt), UpdatedAt: pgTimeToProto(row.UpdatedAt)},
	}
}

func structuredDataGrantToProto(row dbsqlc.StructuredDataGrant) *privatev1.StructuredDataGrant {
	return &privatev1.StructuredDataGrant{
		Id:               row.ID.String(),
		SpaceId:          row.SpaceID.String(),
		TargetType:       row.TargetType,
		TargetBotId:      uuidString(row.TargetBotID),
		TargetBotGroupId: uuidString(row.TargetBotGroupID),
		Privileges:       append([]string(nil), row.Privileges...),
		CreatedByUserId:  uuidString(row.CreatedByUserID),
		Audit:            &privatev1.AuditFields{CreatedAt: pgTimeToProto(row.CreatedAt), UpdatedAt: pgTimeToProto(row.UpdatedAt)},
	}
}

func structuredDataTablesToProto(tables []structureddata.Table) []*privatev1.StructuredDataTable {
	out := make([]*privatev1.StructuredDataTable, 0, len(tables))
	for _, table := range tables {
		columns := make([]*privatev1.StructuredDataColumn, 0, len(table.Columns))
		for _, column := range table.Columns {
			columns = append(columns, &privatev1.StructuredDataColumn{
				Name:         column.Name,
				Type:         column.Type,
				Nullable:     column.Nullable,
				DefaultValue: column.DefaultValue,
			})
		}
		out = append(out, &privatev1.StructuredDataTable{
			SchemaName: table.SchemaName,
			Name:       table.Name,
			Columns:    columns,
		})
	}
	return out
}

func structuredDataSQLResultToProto(result structureddata.SQLResult) *privatev1.StructuredDataSqlResult {
	rows := make([]*structpb.Struct, 0, len(result.Rows))
	for _, row := range result.Rows {
		rows = append(rows, mapToStruct(row))
	}
	return &privatev1.StructuredDataSqlResult{
		Columns:    append([]string(nil), result.Columns...),
		Rows:       rows,
		RowCount:   result.RowCount,
		CommandTag: result.CommandTag,
		Truncated:  result.Truncated,
		SchemaName: result.SchemaName,
	}
}
