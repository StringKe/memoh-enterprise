package connectapi

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/schedule"
)

func TestScheduleToProtoMapsDomainFields(t *testing.T) {
	maxCalls := 7
	createdAt := time.Date(2026, 5, 5, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)

	got := scheduleToProto(schedule.Schedule{
		ID:           "schedule-1",
		BotID:        "bot-1",
		Name:         "Daily review",
		Description:  "Review context",
		Pattern:      "0 9 * * *",
		Command:      "Summarize yesterday",
		MaxCalls:     &maxCalls,
		CurrentCalls: 3,
		Enabled:      true,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	})

	if got.GetId() != "schedule-1" || got.GetBotId() != "bot-1" {
		t.Fatalf("ids = %q/%q", got.GetId(), got.GetBotId())
	}
	if got.GetCron() != "0 9 * * *" || got.GetPrompt() != "Summarize yesterday" {
		t.Fatalf("cron/prompt = %q/%q", got.GetCron(), got.GetPrompt())
	}
	metadata := got.GetMetadata().AsMap()
	if metadata["description"] != "Review context" {
		t.Fatalf("description metadata = %#v", metadata["description"])
	}
	if metadata["max_calls"] != float64(7) {
		t.Fatalf("max_calls metadata = %#v", metadata["max_calls"])
	}
	if metadata["current_calls"] != float64(3) {
		t.Fatalf("current_calls metadata = %#v", metadata["current_calls"])
	}
	if got.GetAudit().GetCreatedAt().AsTime() != createdAt {
		t.Fatalf("created_at = %v", got.GetAudit().GetCreatedAt().AsTime())
	}
	if got.GetAudit().GetUpdatedAt().AsTime() != updatedAt {
		t.Fatalf("updated_at = %v", got.GetAudit().GetUpdatedAt().AsTime())
	}
}

func TestSchedulePagination(t *testing.T) {
	limit, offset, err := schedulePagination(&privatev1.PageRequest{
		PageSize:  250,
		PageToken: "25",
	})
	if err != nil {
		t.Fatal(err)
	}
	if limit != scheduleMaxLimit || offset != 25 {
		t.Fatalf("limit/offset = %d/%d", limit, offset)
	}

	_, _, err = schedulePagination(&privatev1.PageRequest{PageToken: "-1"})
	if err == nil {
		t.Fatal("expected negative page token error")
	}
}

func TestScheduleMetadataInputs(t *testing.T) {
	metadata, err := structpb.NewStruct(map[string]any{
		"description": "Run report",
		"max_calls":   4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := scheduleDescriptionFromMetadata(metadata, "fallback"); got != "Run report" {
		t.Fatalf("description = %q", got)
	}
	maxCalls := scheduleMaxCallsFromMetadata(metadata)
	if !maxCalls.Set || maxCalls.Value == nil || *maxCalls.Value != 4 {
		t.Fatalf("max_calls = %#v", maxCalls)
	}

	nullMetadata, err := structpb.NewStruct(map[string]any{"max_calls": nil})
	if err != nil {
		t.Fatal(err)
	}
	nullMaxCalls := scheduleMaxCallsFromMetadata(nullMetadata)
	if !nullMaxCalls.Set || nullMaxCalls.Value != nil {
		t.Fatalf("null max_calls = %#v", nullMaxCalls)
	}
}
