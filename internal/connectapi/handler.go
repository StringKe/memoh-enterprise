package connectapi

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	iamauth "github.com/memohai/memoh/internal/iam/auth"
)

type Handler struct {
	path    string
	handler http.Handler
}

func NewHandler(path string, handler http.Handler) Handler {
	return Handler{path: path, handler: handler}
}

func (h Handler) Register(e *echo.Echo) {
	e.Any("/connect"+h.path+"*", func(c echo.Context) error {
		userID, err := auth.UserIDFromContext(c)
		req := c.Request()
		if err == nil {
			req = req.WithContext(WithUserID(req.Context(), userID))
		}
		sessionID, err := iamauth.SessionIDFromContext(c)
		if err == nil {
			req = req.WithContext(WithSessionID(req.Context(), sessionID))
		}
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/connect")
		req.URL.RawPath = strings.TrimPrefix(req.URL.RawPath, "/connect")
		h.handler.ServeHTTP(c.Response(), req)
		return nil
	})
}
