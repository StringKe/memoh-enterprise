package connectapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/weixin"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/iam/rbac"
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

func TestChannelServiceStartWeixinQRLogin(t *testing.T) {
	t.Parallel()

	registry := channel.NewRegistry()
	registry.MustRegister(fakeChannelAdapter{channelType: channel.ChannelTypeWeixin, name: "WeChat"})
	service := NewChannelService(nil, registry, nil, nil)
	service.permissions = fakeChannelPermissionChecker{allowed: true}
	service.SetWeixinQRClient(&fakeWeixinQRClient{
		qr: &weixin.QRCodeResponse{
			QRCode:           "qr-login-id",
			QRCodeImgContent: "aW1hZ2U=",
		},
	})

	resp, err := service.StartChannelQrLogin(WithUserID(context.Background(), "user-1"), connect.NewRequest(&privatev1.StartChannelQrLoginRequest{
		BotId:   "bot-1",
		Channel: "weixin",
	}))
	if err != nil {
		t.Fatalf("StartChannelQrLogin returned error: %v", err)
	}
	if resp.Msg.GetLoginId() != "qr-login-id" || resp.Msg.GetQrUrl() != "qr-login-id" {
		t.Fatalf("qr response = (%q, %q), want qr-login-id", resp.Msg.GetLoginId(), resp.Msg.GetQrUrl())
	}
	if string(resp.Msg.GetQrImage()) != "image" {
		t.Fatalf("qr image = %q, want image", string(resp.Msg.GetQrImage()))
	}
	if resp.Msg.GetMimeType() != "image/png" {
		t.Fatalf("mime type = %q, want image/png", resp.Msg.GetMimeType())
	}
}

func TestChannelServicePollWeixinQRLoginSavesConfirmedConfig(t *testing.T) {
	t.Parallel()

	registry := channel.NewRegistry()
	registry.MustRegister(fakeChannelAdapter{channelType: channel.ChannelTypeWeixin, name: "WeChat"})
	store := &fakeChannelLifecycleStore{}
	service := NewChannelService(nil, registry, channel.NewLifecycle(store, fakeChannelConnectionController{}), nil)
	service.permissions = fakeChannelPermissionChecker{allowed: true}
	service.SetWeixinQRClient(&fakeWeixinQRClient{
		status: &weixin.QRStatusResponse{
			Status:      "confirmed",
			BotToken:    "bot-token",
			ILinkBotID:  "ilink-bot",
			ILinkUserID: "ilink-user",
			BaseURL:     "https://weixin.example",
		},
	})

	resp, err := service.PollChannelQrLogin(WithUserID(context.Background(), "user-1"), connect.NewRequest(&privatev1.PollChannelQrLoginRequest{
		BotId:   "bot-1",
		Channel: "weixin",
		LoginId: "qr-login-id",
	}))
	if err != nil {
		t.Fatalf("PollChannelQrLogin returned error: %v", err)
	}
	if resp.Msg.GetStatus() != "confirmed" {
		t.Fatalf("status = %q, want confirmed", resp.Msg.GetStatus())
	}
	if store.upsertBotID != "bot-1" || store.upsertChannel != channel.ChannelTypeWeixin {
		t.Fatalf("upsert target = (%q, %q)", store.upsertBotID, store.upsertChannel)
	}
	if got := store.upsertReq.Credentials["token"]; got != "bot-token" {
		t.Fatalf("saved token = %#v, want bot-token", got)
	}
	if got := store.upsertReq.Credentials["baseUrl"]; got != "https://weixin.example" {
		t.Fatalf("saved baseUrl = %#v, want https://weixin.example", got)
	}
	if resp.Msg.GetConfig().GetChannel() != "weixin" {
		t.Fatalf("config channel = %q, want weixin", resp.Msg.GetConfig().GetChannel())
	}
}

func TestChannelServiceQRLoginRejectsUnsupportedChannel(t *testing.T) {
	t.Parallel()

	registry := channel.NewRegistry()
	registry.MustRegister(fakeChannelAdapter{channelType: "telegram", name: "Telegram"})
	service := NewChannelService(nil, registry, nil, nil)
	service.permissions = fakeChannelPermissionChecker{allowed: true}

	_, err := service.StartChannelQrLogin(WithUserID(context.Background(), "user-1"), connect.NewRequest(&privatev1.StartChannelQrLoginRequest{
		BotId:   "bot-1",
		Channel: "telegram",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("error code = %v, want invalid_argument", connect.CodeOf(err))
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

type fakeChannelPermissionChecker struct {
	allowed bool
	err     error
}

func (f fakeChannelPermissionChecker) HasBotPermission(context.Context, string, string, rbac.PermissionKey) (bool, error) {
	return f.allowed, f.err
}

type fakeWeixinQRClient struct {
	qr     *weixin.QRCodeResponse
	status *weixin.QRStatusResponse
	err    error
}

func (f *fakeWeixinQRClient) FetchQRCode(context.Context, string) (*weixin.QRCodeResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.qr, nil
}

func (f *fakeWeixinQRClient) PollQRStatus(context.Context, string, string) (*weixin.QRStatusResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.status, nil
}

type fakeChannelLifecycleStore struct {
	upsertBotID   string
	upsertChannel channel.ChannelType
	upsertReq     channel.UpsertConfigRequest
}

func (*fakeChannelLifecycleStore) ResolveEffectiveConfig(context.Context, string, channel.ChannelType) (channel.ChannelConfig, error) {
	return channel.ChannelConfig{}, channel.ErrChannelConfigNotFound
}

func (s *fakeChannelLifecycleStore) UpsertConfig(_ context.Context, botID string, channelType channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error) {
	s.upsertBotID = botID
	s.upsertChannel = channelType
	s.upsertReq = req
	now := time.Now().UTC()
	return channel.ChannelConfig{
		ID:          "cfg-1",
		BotID:       botID,
		ChannelType: channelType,
		Credentials: req.Credentials,
		Disabled:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (*fakeChannelLifecycleStore) UpdateConfigDisabled(context.Context, string, channel.ChannelType, bool) (channel.ChannelConfig, error) {
	return channel.ChannelConfig{}, nil
}

func (*fakeChannelLifecycleStore) DeleteConfig(context.Context, string, channel.ChannelType) error {
	return nil
}

type fakeChannelConnectionController struct{}

func (fakeChannelConnectionController) EnsureConnection(context.Context, channel.ChannelConfig) error {
	return nil
}

func (fakeChannelConnectionController) RemoveConnection(context.Context, string, channel.ChannelType) {
}
