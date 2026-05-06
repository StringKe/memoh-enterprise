# Integration WebSocket

企业外部集成只使用 WebSocket 协议。REST webhook 不属于外部 integration 能力面。

## 协议范围

- 外部 integration proto 与内部 Web 管理 proto 分离。
- client 使用企业 integration API token 认证。
- token scope 支持 global、bot 和 bot-group。
- 消息包含 request/response correlation id、ack id、typed events 和 protocol errors。
- 首批 SDK 只提供 Go 和 TypeScript。

## Token 模型

在 Web UI 中创建 integration token。原始 token 只在创建后显示一次。集成运行环境需要把它存入 secret store，并在 WebSocket auth handshake 中发送。

## SDK

- Go SDK: `sdk/go/integration`。
- TypeScript SDK: `packages/integration-sdk-ts`。

SDK 封装连接建立、重连、请求关联、事件迭代、ack、发送 bot message、session 操作、bot status 查询和 server-side action request。
