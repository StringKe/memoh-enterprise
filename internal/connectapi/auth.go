package connectapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/boot"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/handlers"
	iamauth "github.com/memohai/memoh/internal/iam/auth"
	"github.com/memohai/memoh/internal/iam/sso"
)

type AuthService struct {
	accounts     *accounts.Service
	sessionStore handlers.AuthSessionStore
	sso          interface {
		ExchangeLoginCode(ctx context.Context, code string) (sso.LoginCode, error)
	}
	jwtSecret string
	expiresIn time.Duration
	logger    *slog.Logger
}

func NewAuthService(log *slog.Logger, accountService *accounts.Service, queries dbstore.Queries, rc *boot.RuntimeConfig) *AuthService {
	if log == nil {
		log = slog.Default()
	}
	return &AuthService{
		accounts:     accountService,
		sessionStore: handlers.NewDBAuthSessionStore(queries),
		sso:          sso.NewAuthService(queries, rc.JwtExpiresIn, "/login/sso/callback"),
		jwtSecret:    rc.JwtSecret,
		expiresIn:    rc.JwtExpiresIn,
		logger:       log.With(slog.String("service", "connect_auth")),
	}
}

func NewAuthHandler(service *AuthService) Handler {
	path, handler := privatev1connect.NewAuthServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *AuthService) Login(ctx context.Context, req *connect.Request[privatev1.LoginRequest]) (*connect.Response[privatev1.LoginResponse], error) {
	if s.accounts == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("account service not configured"))
	}
	if err := s.checkTokenConfig(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	username := strings.TrimSpace(req.Msg.GetUsername())
	password := strings.TrimSpace(req.Msg.GetPassword())
	if username == "" || password == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("username and password are required"))
	}
	account, err := s.accounts.Login(ctx, username, password)
	if err != nil {
		return nil, authConnectError(err)
	}
	session, err := s.sessionStore.CreateSession(ctx, handlers.AuthSessionInput{
		UserID:    account.ID,
		ExpiresAt: time.Now().UTC().Add(s.expiresIn),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	token, expiresAt, err := iamauth.GenerateToken(account.ID, session.ID, s.jwtSecret, s.expiresIn)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(loginResponse(account, token, session.ID, expiresAt)), nil
}

func (s *AuthService) GetMe(ctx context.Context, _ *connect.Request[privatev1.GetMeRequest]) (*connect.Response[privatev1.GetMeResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	account, err := s.accounts.Get(ctx, userID)
	if err != nil {
		return nil, authConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetMeResponse{User: currentUserToProto(account)}), nil
}

func (s *AuthService) Refresh(ctx context.Context, _ *connect.Request[privatev1.RefreshRequest]) (*connect.Response[privatev1.RefreshResponse], error) {
	if err := s.checkTokenConfig(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	sessionID, err := SessionIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.sessionStore.ValidateSession(ctx, userID, sessionID); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
	}
	token, expiresAt, err := iamauth.GenerateToken(userID, sessionID, s.jwtSecret, s.expiresIn)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.RefreshResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   timeToProto(expiresAt),
	}), nil
}

func (s *AuthService) Logout(ctx context.Context, _ *connect.Request[privatev1.LogoutRequest]) (*connect.Response[privatev1.LogoutResponse], error) {
	sessionID, err := SessionIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.sessionStore.RevokeSession(ctx, sessionID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.LogoutResponse{}), nil
}

func (s *AuthService) ExchangeSsoCode(ctx context.Context, req *connect.Request[privatev1.ExchangeSsoCodeRequest]) (*connect.Response[privatev1.ExchangeSsoCodeResponse], error) {
	if err := s.checkTokenConfig(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	code := strings.TrimSpace(req.Msg.GetCode())
	if code == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("code is required"))
	}
	loginCode, err := s.sso.ExchangeLoginCode(ctx, code)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.sessionStore.ValidateSession(ctx, loginCode.UserID, loginCode.SessionID); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
	}
	token, expiresAt, err := iamauth.GenerateToken(loginCode.UserID, loginCode.SessionID, s.jwtSecret, s.expiresIn)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.ExchangeSsoCodeResponse{Login: &privatev1.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   timeToProto(expiresAt),
		UserId:      loginCode.UserID,
		SessionId:   loginCode.SessionID,
	}}), nil
}

func (s *AuthService) checkTokenConfig() error {
	if strings.TrimSpace(s.jwtSecret) == "" {
		return errors.New("jwt secret not configured")
	}
	if s.expiresIn <= 0 {
		return errors.New("jwt expiry not configured")
	}
	if s.sessionStore == nil {
		return errors.New("session store not configured")
	}
	return nil
}

func loginResponse(account accounts.Account, token, sessionID string, expiresAt time.Time) *privatev1.LoginResponse {
	return &privatev1.LoginResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   timeToProto(expiresAt),
		UserId:      account.ID,
		SessionId:   sessionID,
		DisplayName: account.DisplayName,
		Username:    account.Username,
		Timezone:    account.Timezone,
	}
}

func currentUserToProto(account accounts.Account) *privatev1.CurrentUser {
	return &privatev1.CurrentUser{
		Id:          account.ID,
		Username:    account.Username,
		Email:       account.Email,
		DisplayName: account.DisplayName,
		AvatarUrl:   account.AvatarURL,
		Timezone:    account.Timezone,
		IsActive:    account.IsActive,
		CreatedAt:   timeToProto(account.CreatedAt),
		UpdatedAt:   timeToProto(account.UpdatedAt),
	}
}

func authConnectError(err error) error {
	switch {
	case errors.Is(err, accounts.ErrInvalidCredentials):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
	case errors.Is(err, accounts.ErrInactiveAccount):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("user is inactive"))
	default:
		return connectError(err)
	}
}
