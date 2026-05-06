package connectapi

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
)

func TestContainerRuntimeLifecycleMetricsAndSnapshots(t *testing.T) {
	client, provider := newContainerRuntimeTestClient(t)

	start, err := client.StartContainer(context.Background(), connect.NewRequest(&privatev1.StartContainerRequest{
		BotId:   "bot-1",
		Options: mustStruct(t, map[string]any{"image": "ubuntu:24.04"}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if start.Msg.GetOperation().GetOperationType() != "start_container" {
		t.Fatalf("start operation = %#v", start.Msg.GetOperation())
	}

	lifecycle, err := client.GetContainerLifecycle(context.Background(), connect.NewRequest(&privatev1.GetContainerLifecycleRequest{
		BotId: "bot-1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.Msg.GetBackend() != "local" || lifecycle.Msg.GetStatus() != "available" {
		t.Fatalf("lifecycle = %#v", lifecycle.Msg)
	}

	metrics, err := client.GetContainerMetrics(context.Background(), connect.NewRequest(&privatev1.GetContainerMetricsRequest{
		BotId: "bot-1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Msg.GetCpuPercent() != 12.5 || metrics.Msg.GetMemoryBytes() != 1024 || metrics.Msg.GetMemoryLimitBytes() != 4096 {
		t.Fatalf("metrics = %#v", metrics.Msg)
	}

	created, err := client.CreateContainerSnapshot(context.Background(), connect.NewRequest(&privatev1.CreateContainerSnapshotRequest{
		BotId:    "bot-1",
		Name:     "manual",
		Metadata: &structpb.Struct{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if created.Msg.GetSnapshot().GetSnapshotId() != "7" || created.Msg.GetSnapshot().GetName() != "Manual" {
		t.Fatalf("created snapshot = %#v", created.Msg.GetSnapshot())
	}

	list, err := client.ListContainerSnapshots(context.Background(), connect.NewRequest(&privatev1.ListContainerSnapshotsRequest{
		BotId: "bot-1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := list.Msg.GetSnapshots(); len(got) != 1 || got[0].GetSnapshotId() != "7" {
		t.Fatalf("snapshots = %#v", got)
	}

	if _, err := client.RestoreContainerSnapshot(context.Background(), connect.NewRequest(&privatev1.RestoreContainerSnapshotRequest{
		BotId:      "bot-1",
		SnapshotId: "7",
	})); err != nil {
		t.Fatal(err)
	}
	if provider.rollback != 7 {
		t.Fatalf("rollback version = %d", provider.rollback)
	}

	if _, err := client.DeleteContainerSnapshot(context.Background(), connect.NewRequest(&privatev1.DeleteContainerSnapshotRequest{
		BotId:      "bot-1",
		SnapshotId: "7",
	})); err != nil {
		t.Fatal(err)
	}

	if _, err := client.StopContainer(context.Background(), connect.NewRequest(&privatev1.StopContainerRequest{
		BotId:  "bot-1",
		Reason: "test",
	})); err != nil {
		t.Fatal(err)
	}
	if !provider.stopped {
		t.Fatal("StopContainer did not call runtime StopBot")
	}
}
