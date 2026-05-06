package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	iamauth "github.com/memohai/memoh/internal/iam/auth"
)

func TestShouldSkipJWT_ChannelWebhookPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/channels/feishu/webhook/cfg-1", want: true},
		{path: "/channels/wechatoa/webhook/cfg-1", want: true},
		{path: "/channels/feishu/webhook", want: false},
		{path: "/api/channels/feishu/webhook", want: false},
	}

	for _, tc := range cases {
		got := shouldSkipJWT(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}

func TestShouldSkipJWT_ConnectAuthPublicPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/connect/memoh.private.v1.AuthService/Login", want: true},
		{path: "/connect/memoh.private.v1.AuthService/ExchangeSsoCode", want: true},
		{path: "/connect/memoh.private.v1.AuthService/GetMe", want: false},
		{path: "/connect/memoh.private.v1.AuthService/Refresh", want: false},
		{path: "/connect/memoh.private.v1.AuthService/Logout", want: false},
		{path: "/connect/memoh.private.v1.BotGroupService/ListBotGroups", want: false},
	}

	for _, tc := range cases {
		got := shouldSkipJWT(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}

func TestConnectProtectedRouteRequiresValidToken(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	protected := testHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := NewServer(slog.Default(), ":0", secret, protected)

	cases := []struct {
		name   string
		token  string
		status int
	}{
		{name: "missing token", status: http.StatusUnauthorized},
		{name: "invalid token", token: "invalid", status: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/connect/memoh.private.v1.TestService/Ping", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()

			srv.echo.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}

func TestConnectProtectedRouteAcceptsValidToken(t *testing.T) {
	t.Parallel()

	const secret = "test-secret"
	token, _, err := iamauth.GenerateToken("user-1", "session-1", secret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	protected := testHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := NewServer(slog.Default(), ":0", secret, protected)
	req := httptest.NewRequest(http.MethodPost, "/connect/memoh.private.v1.TestService/Ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestSwaggerRouteIsNotMounted(t *testing.T) {
	t.Parallel()

	srv := NewServer(slog.Default(), ":0", "secret")
	req := httptest.NewRequest(http.MethodGet, "/api/swagger.json", nil)
	rec := httptest.NewRecorder()

	srv.echo.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want not found", rec.Code)
	}
}

type testHandlerFunc func(http.ResponseWriter, *http.Request)

func (f testHandlerFunc) Register(e *echo.Echo) {
	e.Any("/connect/memoh.private.v1.TestService/Ping", func(c echo.Context) error {
		f(c.Response(), c.Request())
		return nil
	})
}
