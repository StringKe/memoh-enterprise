package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/config"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/eventbus"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/settings"
)

type CompactionHandler struct {
	service          *compaction.Service
	botService       *bots.Service
	accountService   *accounts.Service
	settingsService  *settings.Service
	modelsService    *models.Service
	queries          dbstore.Queries
	providersService *providers.Service
	events           *eventbus.Producer
	workerConsumer   string
	logger           *slog.Logger
}

func NewCompactionHandler(
	log *slog.Logger,
	service *compaction.Service,
	botService *bots.Service,
	accountService *accounts.Service,
	settingsService *settings.Service,
	modelsService *models.Service,
	queries dbstore.Queries,
	providersService *providers.Service,
	events *eventbus.Producer,
	cfg config.Config,
) *CompactionHandler {
	workerConsumer := strings.TrimSpace(cfg.Internal.WorkerName)
	if workerConsumer == "" {
		workerConsumer = "memoh-worker"
	}
	return &CompactionHandler{
		service:          service,
		botService:       botService,
		accountService:   accountService,
		settingsService:  settingsService,
		modelsService:    modelsService,
		queries:          queries,
		providersService: providersService,
		events:           events,
		workerConsumer:   workerConsumer,
		logger:           log.With(slog.String("handler", "compaction")),
	}
}

func (h *CompactionHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/compaction")
	group.GET("/logs", h.ListLogs)
	group.DELETE("/logs", h.DeleteLogs)
	e.POST("/bots/:bot_id/sessions/:session_id/compact", h.TriggerCompact)
}

// ListLogs godoc
// @Summary List compaction logs
// @Description List compaction logs for a bot
// @Tags compaction
// @Param bot_id path string true "Bot ID"
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} compaction.ListLogsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/compaction/logs [get].
func (h *CompactionHandler) ListLogs(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	limit, offset := parseOffsetLimit(c)
	items, total, err := h.service.ListLogs(c.Request().Context(), botID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, compaction.ListLogsResponse{Items: items, TotalCount: total})
}

// DeleteLogs godoc
// @Summary Delete compaction logs
// @Description Delete all compaction logs for a bot
// @Tags compaction
// @Param bot_id path string true "Bot ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/compaction/logs [delete].
func (h *CompactionHandler) DeleteLogs(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	if err := h.service.DeleteLogs(c.Request().Context(), botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// TriggerCompactResponse is the API response for triggering compaction.
type TriggerCompactResponse struct {
	Status       string `json:"status"`
	Summary      string `json:"summary,omitempty"`
	MessageCount int    `json:"message_count"`
}

// TriggerCompact godoc
// @Summary Trigger immediate context compaction
// @Description Run context compaction synchronously for a session
// @Tags compaction
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} TriggerCompactResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id}/compact [post].
func (h *CompactionHandler) TriggerCompact(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}

	if h.events == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "eventbus producer is not configured")
	}
	payload, err := json.Marshal(struct {
		BotID     string
		SessionID string
	}{
		BotID:     botID,
		SessionID: sessionID,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if _, err := h.events.Publish(c.Request().Context(), eventbus.Event{
		Topic:          "worker.compaction.run",
		PayloadType:    "memoh.compaction.TriggerConfig",
		Payload:        payload,
		PayloadJSON:    payload,
		IdempotencyKey: "bot-session-compact:" + uuid.NewString(),
		AggregateType:  "bot_session",
		PartitionKey:   sessionID,
	}, []string{h.workerConsumer}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, TriggerCompactResponse{
		Status: "queued",
	})
}

func (*CompactionHandler) requireUserID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *CompactionHandler) authorizeBotAccess(ctx context.Context, userID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, userID, botID)
}
