package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type BotService struct {
	bots *bots.Service
}

func NewBotService(bots *bots.Service) *BotService {
	return &BotService{bots: bots}
}

func NewBotHandler(service *BotService) Handler {
	path, handler := privatev1connect.NewBotServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *BotService) CreateBot(ctx context.Context, req *connect.Request[privatev1.CreateBotRequest]) (*connect.Response[privatev1.CreateBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.Create(ctx, userID, bots.CreateBotRequest{
		DisplayName: req.Msg.GetDisplayName(),
		GroupID:     req.Msg.GetGroupId(),
		AvatarURL:   req.Msg.GetAvatarUrl(),
		Timezone:    req.Msg.Timezone,
		IsActive:    req.Msg.IsActive,
		AclPreset:   req.Msg.GetAclPreset(),
		Metadata:    structToMap(req.Msg.GetMetadata()),
	})
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateBotResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) GetBot(ctx context.Context, req *connect.Request[privatev1.GetBotRequest]) (*connect.Response[privatev1.GetBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.AuthorizeAccess(ctx, userID, req.Msg.GetId(), false)
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) ListBots(ctx context.Context, _ *connect.Request[privatev1.ListBotsRequest]) (*connect.Response[privatev1.ListBotsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	items, err := s.bots.ListByOwner(ctx, userID)
	if err != nil {
		return nil, botConnectError(err)
	}
	out := make([]*privatev1.Bot, 0, len(items))
	for _, item := range items {
		out = append(out, botToProto(item))
	}
	return connect.NewResponse(&privatev1.ListBotsResponse{
		Bots: out,
		Page: &privatev1.PageResponse{},
	}), nil
}

func (s *BotService) UpdateBot(ctx context.Context, req *connect.Request[privatev1.UpdateBotRequest]) (*connect.Response[privatev1.UpdateBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	bot, err := s.bots.Update(ctx, req.Msg.GetId(), bots.UpdateBotRequest{
		DisplayName: req.Msg.DisplayName,
		GroupID:     req.Msg.GroupId,
		AvatarURL:   req.Msg.AvatarUrl,
		Timezone:    req.Msg.Timezone,
		IsActive:    req.Msg.IsActive,
		Metadata:    structToMap(req.Msg.GetMetadata()),
	})
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateBotResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) DeleteBot(ctx context.Context, req *connect.Request[privatev1.DeleteBotRequest]) (*connect.Response[privatev1.DeleteBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetId(), rbac.PermissionBotDelete); err != nil {
		return nil, botConnectError(err)
	}
	if err := s.bots.Delete(ctx, req.Msg.GetId()); err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotResponse{}), nil
}

func (s *BotService) ListBotChecks(ctx context.Context, req *connect.Request[privatev1.ListBotChecksRequest]) (*connect.Response[privatev1.ListBotChecksResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	checks, err := s.bots.ListChecks(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, botConnectError(err)
	}
	out := make([]*privatev1.BotCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, botCheckToProto(check))
	}
	return connect.NewResponse(&privatev1.ListBotChecksResponse{Checks: out}), nil
}

func (s *BotService) AssignBotGroup(ctx context.Context, req *connect.Request[privatev1.AssignBotGroupRequest]) (*connect.Response[privatev1.AssignBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.AssignGroup(ctx, userID, req.Msg.GetBotId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.AssignBotGroupResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) ClearBotGroup(ctx context.Context, req *connect.Request[privatev1.ClearBotGroupRequest]) (*connect.Response[privatev1.ClearBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.ClearGroup(ctx, userID, req.Msg.GetBotId())
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.ClearBotGroupResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) requireBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) error {
	allowed, err := s.bots.HasBotPermission(ctx, userID, botID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return bots.ErrBotAccessDenied
	}
	return nil
}

func botToProto(bot bots.Bot) *privatev1.Bot {
	return &privatev1.Bot{
		Id:                   bot.ID,
		OwnerUserId:          bot.OwnerUserID,
		GroupId:              bot.GroupID,
		DisplayName:          bot.DisplayName,
		AvatarUrl:            bot.AvatarURL,
		Timezone:             bot.Timezone,
		IsActive:             bot.IsActive,
		Status:               bot.Status,
		CheckState:           bot.CheckState,
		CheckIssueCount:      bot.CheckIssueCount,
		SettingsOverrideMask: &privatev1.SettingsOverrideMask{Fields: bot.SettingsOverrideMask},
		Metadata:             mapToStruct(bot.Metadata),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(bot.CreatedAt),
			UpdatedAt: timeToProto(bot.UpdatedAt),
		},
	}
}

func botCheckToProto(check bots.BotCheck) *privatev1.BotCheck {
	return &privatev1.BotCheck{
		Id:       check.ID,
		Type:     check.Type,
		TitleKey: check.TitleKey,
		Subtitle: check.Subtitle,
		Status:   check.Status,
		Summary:  check.Summary,
		Detail:   check.Detail,
		Metadata: mapToStruct(check.Metadata),
	}
}

func botConnectError(err error) error {
	switch {
	case errors.Is(err, bots.ErrBotNotFound), errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, bots.ErrBotAccessDenied), errors.Is(err, bots.ErrBotGroupNotAllowed):
		return connect.NewError(connect.CodePermissionDenied, err)
	default:
		return connectError(err)
	}
}
