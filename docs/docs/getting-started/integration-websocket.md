# Integration WebSocket

Enterprise integrations use a single WebSocket protocol. REST webhooks are not part of the external integration surface.

## Protocol Scope

- External integration proto files are separate from internal Web management proto files.
- Clients authenticate with an enterprise integration API token.
- Token scopes support global, bot, and bot-group access.
- Messages use request/response correlation ids, ack ids, typed events, and protocol errors.
- First-party SDKs are provided for Go and TypeScript.

## Token Model

Create integration tokens from the Web UI. The raw token is shown once after creation. Store it in the integration runtime secret store and send it during the WebSocket auth handshake.

## SDKs

- Go SDK: `sdk/go/integration`.
- TypeScript SDK: `packages/integration-sdk-ts`.

The SDKs wrap connection setup, reconnect, request correlation, event iteration, ack, bot message sending, session operations, bot status queries, and server-side action requests.
