package connectapi

import (
	"errors"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/botgroups"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/settings"
)

func TestConnectErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		code connect.Code
	}{
		{name: "missing user", err: ErrUserIDMissing, code: connect.CodeUnauthenticated},
		{name: "no rows", err: pgx.ErrNoRows, code: connect.CodeNotFound},
		{name: "group not found", err: botgroups.ErrGroupNotFound, code: connect.CodeNotFound},
		{name: "bot access denied", err: bots.ErrBotAccessDenied, code: connect.CodePermissionDenied},
		{name: "invalid model", err: settings.ErrInvalidModelRef, code: connect.CodeInvalidArgument},
		{name: "http unauthorized", err: echo.NewHTTPError(http.StatusUnauthorized, "invalid token"), code: connect.CodeUnauthenticated},
		{name: "http internal", err: echo.NewHTTPError(http.StatusInternalServerError, "failed"), code: connect.CodeInternal},
		{name: "fallback", err: errors.New("bad request"), code: connect.CodeInvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := connectError(tc.err)
			if got := connect.CodeOf(err); got != tc.code {
				t.Fatalf("CodeOf(connectError(%v)) = %v, want %v", tc.err, got, tc.code)
			}
		})
	}
}
