package catalog

import "github.com/memohai/memoh/internal/channel"

type descriptorAdapter struct {
	desc channel.Descriptor
}

func (a descriptorAdapter) Type() channel.ChannelType {
	return a.desc.Type
}

func (a descriptorAdapter) Descriptor() channel.Descriptor {
	return a.desc
}

func RegisterEnterpriseDescriptors(registry *channel.Registry) {
	if registry == nil {
		return
	}
	for _, desc := range enterpriseDescriptors() {
		registry.MustRegister(descriptorAdapter{desc: desc})
	}
}

func enterpriseDescriptors() []channel.Descriptor {
	return []channel.Descriptor{
		descriptor("telegram", "Telegram", true, true, true),
		descriptor("discord", "Discord", true, true, true),
		descriptor("qq", "QQ", true, true, false),
		descriptor("matrix", "Matrix", true, true, true),
		descriptor("feishu", "Feishu", true, true, true),
		descriptor("slack", "Slack", true, true, true),
		descriptor("wecom", "WeCom", true, true, true),
		descriptor("dingtalk", "DingTalk", true, true, false),
		descriptor("wechatoa", "WeChat Official Account", true, false, false),
		descriptor("weixin", "WeChat", true, false, false),
		descriptor("misskey", "Misskey", true, true, false),
	}
}

func descriptor(channelType, displayName string, reply bool, markdown bool, streaming bool) channel.Descriptor {
	return channel.Descriptor{
		Type:        channel.ChannelType(channelType),
		DisplayName: displayName,
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Markdown:       markdown,
			Reply:          reply,
			Attachments:    true,
			Media:          true,
			Streaming:      streaming,
			BlockStreaming: true,
			ChatTypes:      []string{channel.ConversationTypePrivate, channel.ConversationTypeGroup},
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields:  map[string]channel.FieldSchema{},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields:  map[string]channel.FieldSchema{},
		},
		TargetSpec: channel.TargetSpec{
			Format: "external_id",
			Hints: []channel.TargetHint{
				{Label: "External ID", Example: "user-or-channel-id"},
			},
		},
	}
}
