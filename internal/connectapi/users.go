package connectapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/channel/identities"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type UserService struct {
	accounts   *accounts.Service
	identities *identities.Service
	rbac       *rbac.Service
	logger     *slog.Logger
}

func NewUserService(
	log *slog.Logger,
	accountService *accounts.Service,
	identityService *identities.Service,
	rbacService *rbac.Service,
) *UserService {
	if log == nil {
		log = slog.Default()
	}
	return &UserService{
		accounts:   accountService,
		identities: identityService,
		rbac:       rbacService,
		logger:     log.With(slog.String("service", "connect_users")),
	}
}

func NewUserHandler(service *UserService) Handler {
	path, handler := privatev1connect.NewUserServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *UserService) GetCurrentUser(ctx context.Context, _ *connect.Request[privatev1.GetCurrentUserRequest]) (*connect.Response[privatev1.GetCurrentUserResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	account, err := s.accounts.Get(ctx, userID)
	if err != nil {
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetCurrentUserResponse{User: accountToUserProto(account)}), nil
}

func (s *UserService) UpdateCurrentUser(ctx context.Context, req *connect.Request[privatev1.UpdateCurrentUserRequest]) (*connect.Response[privatev1.UpdateCurrentUserResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	account, err := s.accounts.UpdateProfile(ctx, userID, accounts.UpdateProfileRequest{
		DisplayName: optionalString(req.Msg.DisplayName),
		AvatarURL:   optionalString(req.Msg.AvatarUrl),
		Timezone:    optionalString(req.Msg.Timezone),
	})
	if err != nil {
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateCurrentUserResponse{User: accountToUserProto(account)}), nil
}

func (s *UserService) UpdateCurrentUserPassword(ctx context.Context, req *connect.Request[privatev1.UpdateCurrentUserPasswordRequest]) (*connect.Response[privatev1.UpdateCurrentUserPasswordResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.accounts.UpdatePassword(ctx, userID, req.Msg.GetCurrentPassword(), req.Msg.GetNewPassword()); err != nil {
		if errors.Is(err, accounts.ErrInvalidPassword) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("current password mismatch"))
		}
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateCurrentUserPasswordResponse{}), nil
}

func (s *UserService) ListMyIdentities(ctx context.Context, _ *connect.Request[privatev1.ListMyIdentitiesRequest]) (*connect.Response[privatev1.ListMyIdentitiesResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.identities == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("channel identity service not configured"))
	}
	items, err := s.identities.ListUserChannelIdentities(ctx, userID)
	if err != nil {
		return nil, userConnectError(err)
	}
	response := &privatev1.ListMyIdentitiesResponse{
		Identities: make([]*privatev1.UserIdentity, 0, len(items)),
	}
	for _, item := range items {
		response.Identities = append(response.Identities, identityToProto(item))
	}
	return connect.NewResponse(response), nil
}

func (s *UserService) ListUsers(ctx context.Context, _ *connect.Request[privatev1.ListUsersRequest]) (*connect.Response[privatev1.ListUsersResponse], error) {
	if err := s.requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	items, err := s.accounts.ListAccounts(ctx)
	if err != nil {
		return nil, userConnectError(err)
	}
	response := &privatev1.ListUsersResponse{Users: make([]*privatev1.User, 0, len(items))}
	for _, item := range items {
		response.Users = append(response.Users, accountToUserProto(item))
	}
	return connect.NewResponse(response), nil
}

func (s *UserService) GetUser(ctx context.Context, req *connect.Request[privatev1.GetUserRequest]) (*connect.Response[privatev1.GetUserResponse], error) {
	requesterID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	targetID := strings.TrimSpace(req.Msg.GetId())
	if targetID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user id is required"))
	}
	if targetID != requesterID {
		if err := s.requireSystemAdmin(ctx); err != nil {
			return nil, err
		}
	}
	account, err := s.accounts.Get(ctx, targetID)
	if err != nil {
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetUserResponse{User: accountToUserProto(account)}), nil
}

func (s *UserService) CreateUser(ctx context.Context, req *connect.Request[privatev1.CreateUserRequest]) (*connect.Response[privatev1.CreateUserResponse], error) {
	if err := s.requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	active := req.Msg.GetIsActive()
	account, err := s.accounts.CreateHuman(ctx, "", accounts.CreateAccountRequest{
		Username:    req.Msg.GetUsername(),
		Password:    req.Msg.GetPassword(),
		Email:       req.Msg.GetEmail(),
		DisplayName: req.Msg.GetDisplayName(),
		IsActive:    &active,
	})
	if err != nil {
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateUserResponse{User: accountToUserProto(account)}), nil
}

func (s *UserService) UpdateUser(ctx context.Context, req *connect.Request[privatev1.UpdateUserRequest]) (*connect.Response[privatev1.UpdateUserResponse], error) {
	if err := s.requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	account, err := s.accounts.UpdateAdmin(ctx, req.Msg.GetId(), accounts.UpdateAccountRequest{
		DisplayName: optionalString(req.Msg.DisplayName),
		AvatarURL:   optionalString(req.Msg.AvatarUrl),
		IsActive:    req.Msg.IsActive,
	})
	if err != nil {
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateUserResponse{User: accountToUserProto(account)}), nil
}

func (s *UserService) ResetUserPassword(ctx context.Context, req *connect.Request[privatev1.ResetUserPasswordRequest]) (*connect.Response[privatev1.ResetUserPasswordResponse], error) {
	if err := s.requireSystemAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.accounts.ResetPassword(ctx, req.Msg.GetId(), req.Msg.GetNewPassword()); err != nil {
		return nil, userConnectError(err)
	}
	return connect.NewResponse(&privatev1.ResetUserPasswordResponse{}), nil
}

func (s *UserService) requireSystemAdmin(ctx context.Context) error {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.rbac == nil {
		return connect.NewError(connect.CodeInternal, errors.New("rbac service not configured"))
	}
	allowed, err := s.rbac.HasPermission(ctx, rbac.Check{
		UserID:        userID,
		PermissionKey: rbac.PermissionSystemAdmin,
		ResourceType:  rbac.ResourceSystem,
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	if !allowed {
		return connect.NewError(connect.CodePermissionDenied, errors.New("system admin required"))
	}
	return nil
}

func accountToUserProto(account accounts.Account) *privatev1.User {
	return &privatev1.User{
		Id:          account.ID,
		Username:    account.Username,
		Email:       account.Email,
		DisplayName: account.DisplayName,
		AvatarUrl:   account.AvatarURL,
		Timezone:    account.Timezone,
		IsActive:    account.IsActive,
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(account.CreatedAt),
			UpdatedAt: timeToProto(account.UpdatedAt),
		},
	}
}

func identityToProto(item identities.ChannelIdentity) *privatev1.UserIdentity {
	return &privatev1.UserIdentity{
		Id:          item.ID,
		UserId:      item.UserID,
		Channel:     item.Channel,
		ExternalId:  item.ChannelSubjectID,
		DisplayName: item.DisplayName,
		Metadata:    mapToStruct(item.Metadata),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(item.CreatedAt),
			UpdatedAt: timeToProto(item.UpdatedAt),
		},
	}
}

func optionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func userConnectError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return connectError(err)
}
