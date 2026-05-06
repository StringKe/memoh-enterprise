package runnerdispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/runner"
)

func TestStreamChatCreatesLeaseAndStreamsRunnerEvents(t *testing.T) {
	ctx := context.Background()
	botID := "11111111-1111-1111-1111-111111111111"
	userID := "22222222-2222-2222-2222-222222222222"
	sessionID := "33333333-3333-3333-3333-333333333333"
	queries := &fakeLeaseQueries{}
	client, cleanup := newRunnerClient(t)
	defer cleanup()

	service := New(Deps{
		Queries: queries,
		Runner:  client,
		WorkspaceTargets: fakeWorkspaceTargets{
			target: "unix:///run/memoh/workspace-executor.sock",
		},
		Now: func() time.Time {
			return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
		},
	})
	chunks, errs := service.StreamChat(ctx, conversation.ChatRequest{
		BotID:     botID,
		UserID:    userID,
		SessionID: sessionID,
		Query:     "hello",
	})

	var eventTypes []string
	for chunks != nil || errs != nil {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			var event struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(chunk, &event); err != nil {
				t.Fatal(err)
			}
			eventTypes = append(eventTypes, event.Type)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("stream did not finish")
		}
	}
	if queries.created.BotID.String() != botID || queries.created.SessionID.String() != sessionID {
		t.Fatalf("lease params = %#v", queries.created)
	}
	if len(eventTypes) < 2 || eventTypes[0] != "agent_start" || eventTypes[len(eventTypes)-1] != "agent_abort" {
		t.Fatalf("event types = %#v", eventTypes)
	}
}

type fakeLeaseQueries struct {
	created sqlc.CreateAgentRunLeaseParams
}

func (f *fakeLeaseQueries) CreateAgentRunLease(_ context.Context, arg sqlc.CreateAgentRunLeaseParams) (sqlc.AgentRunLease, error) {
	f.created = arg
	return sqlc.AgentRunLease{
		RunID:                     arg.RunID,
		RunnerInstanceID:          arg.RunnerInstanceID,
		BotID:                     arg.BotID,
		BotGroupID:                arg.BotGroupID,
		SessionID:                 arg.SessionID,
		UserID:                    arg.UserID,
		PermissionSnapshotVersion: arg.PermissionSnapshotVersion,
		AllowedToolScopes:         append([]string(nil), arg.AllowedToolScopes...),
		WorkspaceExecutorTarget:   arg.WorkspaceExecutorTarget,
		WorkspaceID:               arg.WorkspaceID,
		ExpiresAt:                 arg.ExpiresAt,
		LeaseVersion:              arg.LeaseVersion,
	}, nil
}

func (*fakeLeaseQueries) GetBotByID(context.Context, pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return sqlc.GetBotByIDRow{}, nil
}

type fakeWorkspaceTargets struct {
	target string
}

func (f fakeWorkspaceTargets) WorkspaceExecutorTarget(string) string {
	return f.target
}

func newRunnerClient(t *testing.T) (runnerv1connect.RunnerServiceClient, func()) {
	t.Helper()
	_, handler := runnerv1connect.NewRunnerServiceHandler(runner.NewService(runner.ServiceDeps{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	return runnerv1connect.NewRunnerServiceClient(server.Client(), server.URL), server.Close
}
