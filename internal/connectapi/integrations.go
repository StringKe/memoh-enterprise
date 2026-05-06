package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/integrations"
)

type IntegrationAdminService struct {
	integrations *integrations.Service
}

func NewIntegrationAdminService(integrations *integrations.Service) *IntegrationAdminService {
	return &IntegrationAdminService{integrations: integrations}
}

func NewIntegrationAdminHandler(service *IntegrationAdminService) Handler {
	path, handler := privatev1connect.NewIntegrationAdminServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *IntegrationAdminService) CreateIntegrationApiToken(ctx context.Context, req *connect.Request[privatev1.CreateIntegrationApiTokenRequest]) (*connect.Response[privatev1.CreateIntegrationApiTokenResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	var expiresAt *time.Time
	if req.Msg.GetExpiresAt() != nil {
		value := req.Msg.GetExpiresAt().AsTime()
		expiresAt = &value
	}
	result, err := s.integrations.CreateAPIToken(ctx, userID, integrations.CreateAPITokenRequest{
		Name:               req.Msg.GetName(),
		ScopeType:          req.Msg.GetScopeType(),
		ScopeBotID:         req.Msg.GetScopeBotId(),
		ScopeBotGroupID:    req.Msg.GetScopeBotGroupId(),
		AllowedEventTypes:  req.Msg.GetAllowedEventTypes(),
		AllowedActionTypes: req.Msg.GetAllowedActionTypes(),
		ExpiresAt:          expiresAt,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&privatev1.CreateIntegrationApiTokenResponse{
		Token:    integrationTokenToProto(result.Token),
		RawToken: result.RawToken,
	}), nil
}

func (s *IntegrationAdminService) ListIntegrationApiTokens(ctx context.Context, _ *connect.Request[privatev1.ListIntegrationApiTokensRequest]) (*connect.Response[privatev1.ListIntegrationApiTokensResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	tokens, err := s.integrations.ListAPITokens(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]*privatev1.IntegrationApiToken, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, integrationTokenToProto(token))
	}
	return connect.NewResponse(&privatev1.ListIntegrationApiTokensResponse{Tokens: items}), nil
}

func (s *IntegrationAdminService) DisableIntegrationApiToken(ctx context.Context, req *connect.Request[privatev1.DisableIntegrationApiTokenRequest]) (*connect.Response[privatev1.DisableIntegrationApiTokenResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	token, err := s.integrations.DisableAPIToken(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&privatev1.DisableIntegrationApiTokenResponse{Token: integrationTokenToProto(token)}), nil
}

func (s *IntegrationAdminService) DeleteIntegrationApiToken(ctx context.Context, req *connect.Request[privatev1.DeleteIntegrationApiTokenRequest]) (*connect.Response[privatev1.DeleteIntegrationApiTokenResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.integrations.DeleteAPIToken(ctx, req.Msg.GetId()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&privatev1.DeleteIntegrationApiTokenResponse{}), nil
}

func (s *IntegrationAdminService) DisableAllIntegrationApiTokens(ctx context.Context, _ *connect.Request[privatev1.DisableAllIntegrationApiTokensRequest]) (*connect.Response[privatev1.DisableAllIntegrationApiTokensResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.integrations.DisableAllAPITokens(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.DisableAllIntegrationApiTokensResponse{}), nil
}

func integrationTokenToProto(token integrations.APIToken) *privatev1.IntegrationApiToken {
	return &privatev1.IntegrationApiToken{
		Id:                 token.ID,
		Name:               token.Name,
		ScopeType:          token.ScopeType,
		ScopeBotId:         token.ScopeBotID,
		ScopeBotGroupId:    token.ScopeBotGroupID,
		AllowedEventTypes:  token.AllowedEventTypes,
		AllowedActionTypes: token.AllowedActionTypes,
		ExpiresAt:          optionalTimeToProto(token.ExpiresAt),
		DisabledAt:         optionalTimeToProto(token.DisabledAt),
		LastUsedAt:         optionalTimeToProto(token.LastUsedAt),
		CreatedByUserId:    token.CreatedByUserID,
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(token.CreatedAt),
			UpdatedAt: timeToProto(token.UpdatedAt),
		},
	}
}

func optionalTimeToProto(value *time.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(*value)
}
