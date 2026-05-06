package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/dingtalk"
	"github.com/memohai/memoh/internal/channel/adapters/discord"
	"github.com/memohai/memoh/internal/channel/adapters/feishu"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/channel/adapters/matrix"
	"github.com/memohai/memoh/internal/channel/adapters/misskey"
	"github.com/memohai/memoh/internal/channel/adapters/qq"
	slackadapter "github.com/memohai/memoh/internal/channel/adapters/slack"
	"github.com/memohai/memoh/internal/channel/adapters/telegram"
	"github.com/memohai/memoh/internal/channel/adapters/wechatoa"
	"github.com/memohai/memoh/internal/channel/adapters/wecom"
	"github.com/memohai/memoh/internal/channel/adapters/weixin"
	"github.com/memohai/memoh/internal/connector"
	"github.com/memohai/memoh/internal/logger"
)

func run(parent context.Context, args []string) error {
	channelType, err := parseChannel(args)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	registry := newConnectorRegistry()
	runtime := connector.NewRuntime(logger.L, registry, connector.NewService(nil), nil)
	logger.L.InfoContext(ctx, "connector runtime ready", slog.String("channel", channelType.String()))

	<-ctx.Done()
	return runtime.Stop(context.WithoutCancel(ctx))
}

func parseChannel(args []string) (channel.ChannelType, error) {
	fs := flag.NewFlagSet("memoh-connector", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	value := fs.String("channel", "", "channel type to run")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	channelType := channel.ChannelType(*value)
	if channelType == "" {
		return "", errors.New("--channel is required")
	}
	return channelType, nil
}

func newConnectorRegistry() *channel.Registry {
	registry := channel.NewRegistry()
	registry.MustRegister(telegram.NewTelegramAdapter(logger.L))
	registry.MustRegister(discord.NewDiscordAdapter(logger.L))
	registry.MustRegister(qq.NewQQAdapter(logger.L))
	registry.MustRegister(matrix.NewMatrixAdapter(logger.L))
	registry.MustRegister(feishu.NewFeishuAdapter(logger.L))
	registry.MustRegister(slackadapter.NewSlackAdapter(logger.L))
	registry.MustRegister(wecom.NewWeComAdapter(logger.L))
	registry.MustRegister(dingtalk.NewDingTalkAdapter(logger.L))
	registry.MustRegister(wechatoa.NewWeChatOAAdapter(logger.L))
	registry.MustRegister(weixin.NewWeixinAdapter(logger.L))
	registry.MustRegister(local.NewWebAdapter(local.NewRouteHub()))
	registry.MustRegister(misskey.NewMisskeyAdapter(logger.L))
	return registry
}
