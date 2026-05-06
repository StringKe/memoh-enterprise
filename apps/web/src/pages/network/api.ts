export interface OverlayConfigSchemaField {
  key: string;
  type: "string" | "secret" | "number" | "bool" | "enum" | "textarea";
  required?: boolean;
  title?: string;
  description?: string;
  placeholder?: string;
  default?: unknown;
  example?: unknown;
  order?: number;
  enum?: string[];
  multiline?: boolean;
  readonly?: boolean;
  secret?: boolean;
  collapsed?: boolean;
  constraint?: { min?: number; max?: number; step?: number } | null;
}

export interface OverlayConfigSchema {
  version?: number;
  title?: string;
  fields?: OverlayConfigSchemaField[];
}

export interface OverlayProviderAction {
  id: string;
  type: string;
  label: string;
  description?: string;
  primary?: boolean;
  status?: { enabled: boolean; reason?: string } | null;
}

export interface WorkspaceRuntimeStatus {
  state: string;
  container_id?: string;
  task_status?: string;
  pid?: number;
  network_target_kind?: string;
  network_target?: string;
  message?: string;
}

export interface NetworkBotStatus {
  provider?: string;
  attached?: boolean;
  state: string;
  title?: string;
  description?: string;
  message?: string;
  network_ip?: string;
  proxy_address?: string;
  details?: Record<string, unknown> | null;
  workspace?: WorkspaceRuntimeStatus | null;
}

export interface OverlayProviderMeta {
  kind: string;
  display_name: string;
  description?: string;
  config_schema?: OverlayConfigSchema;
  binding_config_schema?: OverlayConfigSchema;
  capabilities?: Record<string, boolean>;
  actions?: OverlayProviderAction[];
}

export interface OverlayProviderActionExecution {
  action_id: string;
  status: NetworkBotStatus;
  output?: Record<string, unknown>;
}

export interface NetworkNodeOption {
  id: string;
  value: string;
  display_name: string;
  description?: string;
  online?: boolean;
  addresses?: string[];
  can_exit_node?: boolean;
  selected?: boolean;
  details?: Record<string, unknown> | null;
}

export interface NetworkNodeListResponse {
  provider?: string;
  items?: NetworkNodeOption[];
  message?: string;
}
