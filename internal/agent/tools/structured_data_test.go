package tools

import (
	"context"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

type fakeStructuredDataRuntime struct {
	listCalled bool
	execInput  StructuredDataSQLInput
}

func (f *fakeStructuredDataRuntime) ListStructuredDataSpaces(context.Context) ([]StructuredDataSpace, error) {
	f.listCalled = true
	return []StructuredDataSpace{{ID: "space-1", OwnerType: "bot", SchemaName: "bot_data_test"}}, nil
}

func (f *fakeStructuredDataRuntime) ExecuteStructuredDataSQL(_ context.Context, input StructuredDataSQLInput) (StructuredDataSQLResult, error) {
	f.execInput = input
	return StructuredDataSQLResult{Columns: []string{"ok"}, Rows: []map[string]any{{"ok": true}}, RowCount: 1}, nil
}

func TestStructuredDataProviderTools(t *testing.T) {
	runtime := &fakeStructuredDataRuntime{}
	provider := NewStructuredDataProvider(runtime)
	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tool count = %d, want 2", len(tools))
	}
	if tools[0].Name != "structured_data_spaces" || tools[1].Name != "structured_data_sql" {
		t.Fatalf("tool names = %q, %q", tools[0].Name, tools[1].Name)
	}

	if _, err := tools[0].Execute(&sdk.ToolExecContext{Context: context.Background()}, nil); err != nil {
		t.Fatalf("spaces tool returned error: %v", err)
	}
	if !runtime.listCalled {
		t.Fatalf("spaces tool did not call runtime")
	}

	_, err = tools[1].Execute(&sdk.ToolExecContext{Context: context.Background()}, map[string]any{
		"space_id": "space-1",
		"sql":      "create table items (id bigint);",
		"max_rows": float64(25),
	})
	if err != nil {
		t.Fatalf("sql tool returned error: %v", err)
	}
	if runtime.execInput.SpaceID != "space-1" || runtime.execInput.SQL == "" || runtime.execInput.MaxRows != 25 {
		t.Fatalf("exec input = %#v", runtime.execInput)
	}
}

func TestStructuredDataProviderHiddenForSubagent(t *testing.T) {
	provider := NewStructuredDataProvider(&fakeStructuredDataRuntime{})
	tools, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1", IsSubagent: true})
	if err != nil {
		t.Fatalf("Tools returned error: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tool count = %d, want 0", len(tools))
	}
}
