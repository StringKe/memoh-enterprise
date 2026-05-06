import type { SupermarketItem } from "@stringke/sdk/connect";

export type SupermarketAuthor = {
  name?: string;
  email?: string;
};

export type SupermarketConfigVar = {
  key?: string;
  description?: string;
  defaultValue?: string;
};

export type SupermarketMcpEntry = {
  id: string;
  name: string;
  description: string;
  author?: SupermarketAuthor;
  transport: string;
  icon?: string;
  homepage?: string;
  tags?: string[];
  url?: string;
  command?: string;
  args?: string[];
  headers?: SupermarketConfigVar[];
  env?: SupermarketConfigVar[];
};

export type SupermarketSkillEntry = {
  id: string;
  name: string;
  description: string;
  metadata?: {
    author?: SupermarketAuthor;
    tags?: string[];
    homepage?: string;
  };
  content?: string;
  files?: string[];
};

export function mcpEntryFromConnect(item: SupermarketItem): SupermarketMcpEntry {
  const metadata = structRecord(item.metadata);
  return {
    id: item.id,
    name: item.displayName || item.name,
    description: item.description,
    author: recordValue(metadata.author) as SupermarketAuthor | undefined,
    transport: stringValue(metadata.transport),
    icon: stringValue(metadata.icon),
    homepage: stringValue(metadata.homepage),
    tags: item.tags,
    url: stringValue(metadata.url),
    command: stringValue(metadata.command),
    args: stringArray(metadata.args),
    headers: configVars(metadata.headers),
    env: configVars(metadata.env),
  };
}

export function skillEntryFromConnect(item: SupermarketItem): SupermarketSkillEntry {
  const metadata = structRecord(item.metadata);
  return {
    id: item.id,
    name: item.displayName || item.name,
    description: item.description,
    metadata: {
      author: recordValue(metadata.author) as SupermarketAuthor | undefined,
      tags: item.tags,
      homepage: stringValue(metadata.homepage),
    },
    content: stringValue(metadata.content),
    files: stringArray(metadata.files),
  };
}

function configVars(value: unknown): SupermarketConfigVar[] {
  return arrayValue(value)
    .map((item) => recordValue(item) as SupermarketConfigVar | undefined)
    .filter((item): item is SupermarketConfigVar => Boolean(item));
}

function structRecord(value: unknown): Record<string, unknown> {
  const record = recordValue(value);
  if (!record) return {};
  if (!record.fields || typeof record.fields !== "object") return record;

  const result: Record<string, unknown> = {};
  for (const [key, fieldValue] of Object.entries(record.fields as Record<string, unknown>)) {
    result[key] = protobufValue(fieldValue);
  }
  return result;
}

function protobufValue(value: unknown): unknown {
  const record = recordValue(value);
  if (!record) return value;
  const kind = record.kind as { case?: string; value?: unknown } | undefined;
  if (!kind) return value;
  switch (kind.case) {
    case "structValue":
      return structRecord(kind.value);
    case "listValue":
      return arrayValue((recordValue(kind.value) ?? {}).values).map(protobufValue);
    case "stringValue":
    case "numberValue":
    case "boolValue":
      return kind.value;
    case "nullValue":
      return null;
    default:
      return kind.value;
  }
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function stringArray(value: unknown): string[] {
  return arrayValue(value).filter((item): item is string => typeof item === "string");
}
