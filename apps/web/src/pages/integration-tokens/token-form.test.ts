import { describe, expect, it } from "vitest";

import {
  buildCreateIntegrationApiTokenRequest,
  canCreateIntegrationApiToken,
  getRawTokenDisplay,
  parseTokenCSV,
  type IntegrationTokenFormState,
} from "./token-form";

function form(overrides: Partial<IntegrationTokenFormState> = {}): IntegrationTokenFormState {
  return {
    name: "Enterprise client",
    scopeType: "global",
    scopeBotId: "",
    scopeBotGroupId: "",
    allowedEventTypes: "",
    allowedActionTypes: "",
    expiresAt: "",
    ...overrides,
  };
}

describe("parseTokenCSV", () => {
  it("trims comma-separated scopes and removes empty entries", () => {
    expect(parseTokenCSV("message.created, , bot.status.changed ")).toEqual([
      "message.created",
      "bot.status.changed",
    ]);
  });
});

describe("canCreateIntegrationApiToken", () => {
  it("accepts global tokens with a non-empty name", () => {
    expect(canCreateIntegrationApiToken(form({ name: "  Global token  " }))).toBe(true);
  });

  it("requires a bot id for bot-scoped tokens", () => {
    expect(canCreateIntegrationApiToken(form({ scopeType: "bot" }))).toBe(false);
    expect(canCreateIntegrationApiToken(form({ scopeType: "bot", scopeBotId: "bot-1" }))).toBe(
      true,
    );
  });

  it("requires a bot group id for bot-group-scoped tokens", () => {
    expect(canCreateIntegrationApiToken(form({ scopeType: "bot_group" }))).toBe(false);
    expect(
      canCreateIntegrationApiToken(form({ scopeType: "bot_group", scopeBotGroupId: "group-1" })),
    ).toBe(true);
  });

  it("rejects empty names and unknown scope types", () => {
    expect(canCreateIntegrationApiToken(form({ name: " " }))).toBe(false);
    expect(canCreateIntegrationApiToken(form({ scopeType: "workspace" }))).toBe(false);
  });
});

describe("buildCreateIntegrationApiTokenRequest", () => {
  it("builds a global token payload and clears scoped ids", () => {
    expect(
      buildCreateIntegrationApiTokenRequest(
        form({
          name: "  Global token  ",
          scopeBotId: "bot-1",
          scopeBotGroupId: "group-1",
          allowedEventTypes: "message.created,bot.status.changed",
          allowedActionTypes: "session.create, message.send",
        }),
      ),
    ).toMatchObject({
      name: "Global token",
      scopeType: "global",
      scopeBotId: "",
      scopeBotGroupId: "",
      allowedEventTypes: ["message.created", "bot.status.changed"],
      allowedActionTypes: ["session.create", "message.send"],
      expiresAt: undefined,
    });
  });

  it("builds bot and bot group scoped payloads with only the matching id", () => {
    expect(
      buildCreateIntegrationApiTokenRequest(
        form({ scopeType: "bot", scopeBotId: " bot-1 ", scopeBotGroupId: "group-1" }),
      ),
    ).toMatchObject({
      scopeType: "bot",
      scopeBotId: "bot-1",
      scopeBotGroupId: "",
    });

    expect(
      buildCreateIntegrationApiTokenRequest(
        form({ scopeType: "bot_group", scopeBotId: "bot-1", scopeBotGroupId: " group-1 " }),
      ),
    ).toMatchObject({
      scopeType: "bot_group",
      scopeBotId: "",
      scopeBotGroupId: "group-1",
    });
  });
});

describe("getRawTokenDisplay", () => {
  it("keeps the raw token visible and unmasked after creation", () => {
    expect(getRawTokenDisplay("memoh_it_123.secret")).toEqual({
      visible: true,
      value: "memoh_it_123.secret",
    });
  });

  it("hides the raw token panel before a token is created", () => {
    expect(getRawTokenDisplay("")).toEqual({ visible: false, value: "" });
  });
});
