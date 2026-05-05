# 了解 Memoh

## Memoh 是什么

Memoh Enterprise 是面向多成员、可结构化长期记忆、容器化运行的 AI 智能体平台。你可以创建多个机器人，为每个机器人提供独立工作区和长期记忆，通过 Telegram、Discord、飞书、QQ、Matrix、Misskey、钉钉、企微、微信、微信公众号、邮件或 API/CLI 集成使用。

每个机器人有各自的运行环境、工具、记忆与渠道。用起来更像「每人一台小电脑上的智能体」，而不是所有人共用一条聊天人格。

## 和其它方案不一样在哪

### 多机器人、多用户

- 一个账号里可建多个机器人，分角色或分场景用。
- 私聊、群聊、委派的流程里，人和机器人都可参与。
- 共享对话里能区分不同用户；跨渠道绑定身份后，同一个人可被稳定识别。

### 容器化工作区

每个机器人在自己的容器里跑，有独立文件系统和网络边界。机器人在工作区内读写文件、跑命令、用工具，不会踩到别的机器人。

### 长期记忆与会话负担

这是两件不同的事：

- **长期记忆**通过各记忆提供方存事实、跨会话检索。
- **会话上下文压缩**是在「当前这一串对话」太长时，用摘要把活跃窗口缩小。

注意：压缩会话上下文，改的是当前对话窗口；**记忆压缩**（在记忆 tab）改的是存下来的记忆条目本身。

### 会话与 Discuss 模式

每个机器人有多路 **会话**，自带上下文。Memoh 里常见五类：

- **Chat**：普通面向人的对话。
- **Discuss**：偏观察；模型先在内心里组织语言，只有在你用发送类动作时才算真正对频道说话。
- **Heartbeat**：按间隔自动跑的任务会话。
- **Schedule**：由 cron 触发的任务会话。
- **Subagent**：委派子智能体时产生的会话。

在渠道里可以用 `/new` 等切会话，API 响应可查看上下文占用、缓存命中、用到的技能等。

### 渠道覆盖面

用统一的渠道适配，让一个机器人能同时在多个地方被叫到。当前可对接例如 Telegram、Discord、飞书、QQ、Matrix、Misskey、钉钉、企微、微信、公众号、邮件、Web。

**个人微信扫码**与**公众号 Webhook** 是两套不同适配，别混用。

### 工具、技能、MCP、超市

内置能力包括：网页搜索与拉取、浏览器自动化、工作区内改文件/跑命令、记忆检索与管理、发消息/邮件、TTS、子智能体、可复用 **技能** 模块、外部 **MCP** 服务、以及从 **超市** 装技能和 MCP 模板。

### 供应商与模型

支持多种对接方式，例如：OpenAI 系 Chat/Responses、Anthropic Messages、Google、Codex、GitHub Copilot、Edge 朗读等。模型按 **chat / embedding / speech** 分角色。文生图走兼容的 chat/图像能力，不单独做一层「图像供应商系统」。

### 运维

HTTP API 和 CLI 覆盖机器人、供应商与模型、会话状态、技能显隐、渠道命令等日常操作。

## 从哪开始

- [Docker 安装](/zh/installation/docker) — 先把服务跑起来
- [供应商与模型](/zh/getting-started/provider-and-model) — 配好模型访问
- [机器人](/zh/getting-started/bot) — 创建并配置
- [会话](/zh/getting-started/sessions) — 弄清 chat 与 discuss
- [渠道](/zh/getting-started/channels) — 选机器人出现的位置
- [技能](/zh/getting-started/skills)、[超市](/zh/getting-started/supermarket) — 扩展能力
- [斜杠命令](/zh/getting-started/slash-commands) — 在聊天里直接操控
