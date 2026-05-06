package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/providers"
)

func (s *ProviderService) GetProviderOauthStatus(ctx context.Context, req *connect.Request[privatev1.GetProviderOauthStatusRequest]) (*connect.Response[privatev1.GetProviderOauthStatusResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	status, err := s.providers.GetOAuthStatus(providerOAuthContext(ctx), providerID)
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(providerOAuthStatusToProto(status)), nil
}

func (s *ProviderService) StartProviderOauth(ctx context.Context, req *connect.Request[privatev1.StartProviderOauthRequest]) (*connect.Response[privatev1.StartProviderOauthResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	result, err := s.providers.StartOAuthAuthorization(providerOAuthContext(ctx), providerID)
	if err != nil {
		return nil, providerConnectError(err)
	}
	response := &privatev1.StartProviderOauthResponse{
		AuthorizeUrl: result.AuthURL,
	}
	if result.Device != nil {
		response.AuthorizeUrl = result.Device.VerificationURI
		response.State = result.Device.UserCode
	}
	return connect.NewResponse(response), nil
}

func (s *ProviderService) PollProviderOauth(ctx context.Context, req *connect.Request[privatev1.PollProviderOauthRequest]) (*connect.Response[privatev1.PollProviderOauthResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	status, err := s.providers.PollOAuthAuthorization(providerOAuthContext(ctx), providerID)
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.PollProviderOauthResponse{
		Complete: status.HasToken,
		Status:   providerOAuthStatusToProto(status),
	}), nil
}

func (s *ProviderService) RevokeProviderOauth(ctx context.Context, req *connect.Request[privatev1.RevokeProviderOauthRequest]) (*connect.Response[privatev1.RevokeProviderOauthResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	if err := s.providers.RevokeOAuthToken(providerOAuthContext(ctx), providerID); err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.RevokeProviderOauthResponse{}), nil
}

func providerOAuthStatusToProto(status *providers.OAuthStatus) *privatev1.GetProviderOauthStatusResponse {
	if status == nil {
		return &privatev1.GetProviderOauthStatusResponse{Metadata: mapToStruct(map[string]any{})}
	}
	account := ""
	if status.Account != nil {
		account = firstNonEmptyString(status.Account.Label, status.Account.Login, status.Account.Email, status.Account.Name)
	}
	metadata := map[string]any{
		"configured":   status.Configured,
		"mode":         status.Mode,
		"expired":      status.Expired,
		"callback_url": status.CallbackURL,
	}
	if status.ExpiresAt != nil {
		metadata["expires_at"] = status.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if status.Device != nil {
		metadata["device"] = map[string]any{
			"pending":          status.Device.Pending,
			"user_code":        status.Device.UserCode,
			"verification_uri": status.Device.VerificationURI,
			"interval_seconds": status.Device.IntervalSeconds,
		}
		if status.Device.ExpiresAt != nil {
			metadata["device"].(map[string]any)["expires_at"] = status.Device.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		}
	}
	return &privatev1.GetProviderOauthStatusResponse{
		Authorized: status.HasToken,
		Account:    account,
		Metadata:   mapToStruct(metadata),
	}
}
