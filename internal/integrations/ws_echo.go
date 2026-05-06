package integrations

import (
	"github.com/labstack/echo/v4"
)

func (h *WebSocketHandler) Register(e *echo.Echo) {
	e.GET(WebSocketPath, echo.WrapHandler(h))
}
