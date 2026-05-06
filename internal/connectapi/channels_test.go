package connectapi

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/memohai/memoh/internal/channel"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
)

type fakeChannelAdapter struct {
	channelType channel.ChannelType
	name        string
}

func (a fakeChannelAdapter) Type() channel.ChannelType {
	return a.channelType
}

func (a fakeChannelAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        a.channelType,
		DisplayName: a.name,
		Capabilities: channel.ChannelCapabilities{
			Text: true,
		},
	}
}

type fakeWebhookChannelAdapter struct {
	fakeChannelAdapter
}

func (fakeWebhookChannelAdapter) HandleWebhook(context.Context, channel.ChannelConfig, channel.InboundHandler, *http.Request, http.ResponseWriter) error {
	return nil
}

func TestChannelServiceListChannelsSortsAndMarksWebhook(t *testing.T) {
	t.Parallel()

	registry := channel.NewRegistry()
	registry.MustRegister(fakeWebhookChannelAdapter{fakeChannelAdapter{channelType: "zeta", name: "Zeta"}})
	registry.MustRegister(fakeChannelAdapter{channelType: "alpha", name: "Alpha"})
	service := NewChannelService(nil, registry, nil, nil)

	resp, err := service.ListChannels(WithUserID(context.Background(), "user-1"), connect.NewRequest(&privatev1.ListChannelsRequest{}))
	if err != nil {
		t.Fatalf("ListChannels returned error: %v", err)
	}
	items := resp.Msg.GetChannels()
	if len(items) != 2 {
		t.Fatalf("channels length = %d, want 2", len(items))
	}
	if items[0].GetId() != "alpha" || items[1].GetId() != "zeta" {
		t.Fatalf("channels order = [%s, %s], want [alpha, zeta]", items[0].GetId(), items[1].GetId())
	}
	if items[0].GetSupportsWebhook() {
		t.Fatalf("alpha supports webhook = true, want false")
	}
	if !items[1].GetSupportsWebhook() {
		t.Fatalf("zeta supports webhook = false, want true")
	}
}

func TestChannelIdentityIDForRequestRejectsOtherUser(t *testing.T) {
	t.Parallel()

	if _, err := channelIdentityIDForRequest("user-1", "user-2"); err == nil {
		t.Fatal("expected mismatch error")
	}
	got, err := channelIdentityIDForRequest("user-1", "")
	if err != nil {
		t.Fatalf("empty request identity returned error: %v", err)
	}
	if got != "user-1" {
		t.Fatalf("identity id = %q, want user-1", got)
	}
}

func TestToolApprovalReasonFromPayload(t *testing.T) {
	t.Parallel()

	payload, err := structpb.NewStruct(map[string]any{"reason": " checked "})
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	if got := toolApprovalReasonFromPayload(payload); got != "checked" {
		t.Fatalf("reason = %q, want checked", got)
	}
}

func TestUsagePaginationCapsLimitAndParsesOffset(t *testing.T) {
	t.Parallel()

	limit, offset, err := usagePagination(&privatev1.PageRequest{
		PageSize:  1000,
		PageToken: "40",
	})
	if err != nil {
		t.Fatalf("usagePagination returned error: %v", err)
	}
	if limit != usageRecordsMaxLimit {
		t.Fatalf("limit = %d, want %d", limit, usageRecordsMaxLimit)
	}
	if offset != 40 {
		t.Fatalf("offset = %d, want 40", offset)
	}

	if _, _, err := usagePagination(&privatev1.PageRequest{PageToken: "-1"}); err == nil {
		t.Fatal("expected negative page token error")
	}
}
