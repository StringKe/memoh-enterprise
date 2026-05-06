import { createTimestampFromDate, type TimestampMessage } from "@stringke/sdk/connect";

export type IntegrationTokenScopeType = "global" | "bot" | "bot_group";

export type IntegrationTokenFormState = {
  name: string;
  scopeType: string;
  scopeBotId: string;
  scopeBotGroupId: string;
  allowedEventTypes: string;
  allowedActionTypes: string;
  expiresAt: string;
};

export type CreateIntegrationApiTokenInput = {
  name: string;
  scopeType: IntegrationTokenScopeType;
  scopeBotId: string;
  scopeBotGroupId: string;
  allowedEventTypes: string[];
  allowedActionTypes: string[];
  expiresAt?: TimestampMessage;
};

export type RawTokenDisplay = {
  visible: boolean;
  value: string;
};

export function parseTokenCSV(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function isIntegrationTokenScopeType(value: string): value is IntegrationTokenScopeType {
  return value === "global" || value === "bot" || value === "bot_group";
}

export function canCreateIntegrationApiToken(form: IntegrationTokenFormState): boolean {
  if (!form.name.trim()) return false;
  if (form.scopeType === "bot") return Boolean(form.scopeBotId.trim());
  if (form.scopeType === "bot_group") return Boolean(form.scopeBotGroupId.trim());
  return form.scopeType === "global";
}

export function buildCreateIntegrationApiTokenRequest(
  form: IntegrationTokenFormState,
): CreateIntegrationApiTokenInput {
  const scopeType = isIntegrationTokenScopeType(form.scopeType) ? form.scopeType : "global";

  return {
    name: form.name.trim(),
    scopeType,
    scopeBotId: scopeType === "bot" ? form.scopeBotId.trim() : "",
    scopeBotGroupId: scopeType === "bot_group" ? form.scopeBotGroupId.trim() : "",
    allowedEventTypes: parseTokenCSV(form.allowedEventTypes),
    allowedActionTypes: parseTokenCSV(form.allowedActionTypes),
    expiresAt: form.expiresAt ? createTimestampFromDate(new Date(form.expiresAt)) : undefined,
  };
}

export function getRawTokenDisplay(rawToken: string): RawTokenDisplay {
  return {
    visible: rawToken.length > 0,
    value: rawToken,
  };
}
