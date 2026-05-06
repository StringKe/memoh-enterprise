package connectapi

import (
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/botgroups"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/settings"
)

func connectError(err error) error {
	if err == nil {
		return nil
	}

	var httpErr *echo.HTTPError
	switch {
	case errors.Is(err, ErrUserIDMissing), errors.Is(err, ErrSessionIDMissing):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, botgroups.ErrGroupNotFound), errors.Is(err, bots.ErrBotNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, botgroups.ErrGroupAccessDenied), errors.Is(err, bots.ErrBotAccessDenied), errors.Is(err, bots.ErrBotGroupNotAllowed):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, accounts.ErrInvalidCredentials), errors.Is(err, accounts.ErrInactiveAccount):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, settings.ErrInvalidModelRef):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.As(err, &httpErr):
		return connect.NewError(connectCodeFromHTTPStatus(httpErr.Code), err)
	default:
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
}

func connectCodeFromHTTPStatus(status int) connect.Code {
	switch status {
	case http.StatusBadRequest:
		return connect.CodeInvalidArgument
	case http.StatusUnauthorized:
		return connect.CodeUnauthenticated
	case http.StatusForbidden:
		return connect.CodePermissionDenied
	case http.StatusNotFound:
		return connect.CodeNotFound
	case http.StatusConflict:
		return connect.CodeAlreadyExists
	case http.StatusTooManyRequests:
		return connect.CodeResourceExhausted
	case http.StatusNotImplemented:
		return connect.CodeUnimplemented
	case http.StatusServiceUnavailable:
		return connect.CodeUnavailable
	default:
		if status >= 500 {
			return connect.CodeInternal
		}
		return connect.CodeUnknown
	}
}
