<template>
  <SettingsShell width="wide" class="space-y-6">
    <header class="flex items-start justify-between gap-6">
      <div class="space-y-1">
        <h2 class="text-base font-medium">
          {{ $t("bots.toolApproval.title") }}
        </h2>
        <p class="max-w-2xl text-xs leading-relaxed text-muted-foreground">
          {{ $t("bots.toolApproval.intro") }}
        </p>
        <InheritanceField
          :fields="[FIELD_TOOL_APPROVAL_CONFIG]"
          :sources="effectiveSettings?.sources"
          :loading="restoreLoading"
          @restore="handleRestoreInheritance([FIELD_TOOL_APPROVAL_CONFIG])"
        />
      </div>
      <Switch
        :model-value="form.tool_approval_config.enabled"
        @update:model-value="(val) => (form.tool_approval_config.enabled = !!val)"
      />
    </header>

    <Separator />

    <div
      class="space-y-6 transition-opacity"
      :class="{ 'opacity-60 pointer-events-none': !form.tool_approval_config.enabled }"
      :aria-disabled="!form.tool_approval_config.enabled"
    >
      <template v-for="(tool, idx) in approvalTools" :key="tool">
        <section class="space-y-4">
          <div class="flex items-start justify-between gap-4">
            <div class="flex items-start gap-3">
              <component
                :is="TOOL_META[tool].icon"
                class="size-5 mt-0.5 shrink-0 text-muted-foreground"
              />
              <div class="space-y-0.5">
                <h3 class="font-mono text-sm font-medium">
                  {{ tool }}
                </h3>
                <p class="text-xs text-muted-foreground">
                  {{ $t(TOOL_META[tool].descKey) }}
                </p>
              </div>
            </div>
            <Switch
              :model-value="toolApprovalPolicy(tool).require_approval"
              @update:model-value="(val) => (toolApprovalPolicy(tool).require_approval = !!val)"
            />
          </div>

          <p
            v-if="!toolApprovalPolicy(tool).require_approval"
            class="rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground"
          >
            {{ $t("bots.toolApproval.toolDisabledHint") }}
          </p>

          <div class="grid gap-4 md:grid-cols-2 items-stretch">
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <Label class="flex items-center gap-1.5 text-xs font-medium">
                  <ShieldCheck class="size-3.5 text-emerald-600 dark:text-emerald-500" />
                  {{ $t("bots.toolApproval.bypass") }}
                </Label>
                <span class="text-xs text-muted-foreground tabular-nums">
                  {{ bypassList(tool).length }}
                </span>
              </div>
              <p class="text-xs text-muted-foreground">
                {{ $t("bots.toolApproval.bypassHint") }}
              </p>
              <Textarea
                :model-value="approvalBypassText(tool)"
                :placeholder="bypassPlaceholder(tool)"
                class="min-h-32 flex-1 resize-none font-mono text-xs"
                @update:model-value="(val) => updateApprovalBypass(tool, String(val))"
              />
            </div>

            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <Label class="flex items-center gap-1.5 text-xs font-medium">
                  <ShieldAlert class="size-3.5 text-amber-600 dark:text-amber-500" />
                  {{ $t("bots.toolApproval.mustReview") }}
                </Label>
                <span class="text-xs text-muted-foreground tabular-nums">
                  {{ forceReviewList(tool).length }}
                </span>
              </div>
              <p class="text-xs text-muted-foreground">
                {{ $t("bots.toolApproval.mustReviewHint") }}
              </p>
              <Textarea
                :model-value="approvalForceReviewText(tool)"
                :placeholder="forceReviewPlaceholder(tool)"
                class="min-h-32 flex-1 resize-none font-mono text-xs"
                @update:model-value="(val) => updateApprovalForceReview(tool, String(val))"
              />
            </div>
          </div>
        </section>

        <Separator v-if="idx < approvalTools.length - 1" />
      </template>
    </div>

    <Separator />

    <div class="flex items-center justify-end gap-3 pt-1">
      <span v-if="hasChanges" class="text-xs text-muted-foreground">
        {{ $t("bots.toolApproval.unsavedChanges") }}
      </span>
      <Button size="sm" :disabled="!hasChanges || saveLoading" @click="handleSave">
        <Spinner v-if="saveLoading" class="mr-2 size-4" />
        {{ $t("bots.settings.save") }}
      </Button>
    </div>
  </SettingsShell>
</template>

<script setup lang="ts">
import { Label, Textarea, Button, Spinner, Switch, Separator } from "@stringke/ui";
import { FilePlus2, FilePen, SquareTerminal, ShieldCheck, ShieldAlert } from "lucide-vue-next";
import { reactive, computed, watch } from "vue";
import type { Component, Ref } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useQuery, useMutation, useQueryCache } from "@pinia/colada";
import type { BotSettings } from "@stringke/sdk/connect";
import SettingsShell from "@/components/settings-shell/index.vue";
import { resolveApiErrorMessage } from "@/utils/api-error";
import { connectClients } from "@/lib/connect-client";
import InheritanceField from "./inheritance-field.vue";

const props = defineProps<{
  botId: string;
}>();

type ApprovalTool = "write" | "edit" | "exec";

interface ToolApprovalFilePolicy {
  require_approval: boolean;
  bypass_globs: string[];
  force_review_globs: string[];
}

interface ToolApprovalExecPolicy {
  require_approval: boolean;
  bypass_commands: string[];
  force_review_commands: string[];
}

interface ToolApprovalConfig {
  enabled: boolean;
  write: ToolApprovalFilePolicy;
  edit: ToolApprovalFilePolicy;
  exec: ToolApprovalExecPolicy;
}

const defaultToolApprovalConfig = (): ToolApprovalConfig => ({
  enabled: false,
  write: {
    require_approval: true,
    bypass_globs: ["/data/**", "/tmp/**"],
    force_review_globs: [],
  },
  edit: {
    require_approval: true,
    bypass_globs: ["/data/**", "/tmp/**"],
    force_review_globs: [],
  },
  exec: {
    require_approval: false,
    bypass_commands: [],
    force_review_commands: [],
  },
});

const approvalTools: ApprovalTool[] = ["write", "edit", "exec"];

const TOOL_META: Record<ApprovalTool, { icon: Component; descKey: string }> = {
  write: { icon: FilePlus2, descKey: "bots.toolApproval.tools.write" },
  edit: { icon: FilePen, descKey: "bots.toolApproval.tools.edit" },
  exec: { icon: SquareTerminal, descKey: "bots.toolApproval.tools.exec" },
};

const { t } = useI18n();

const botIdRef = computed(() => props.botId) as Ref<string>;

const queryCache = useQueryCache();

const FIELD_TOOL_APPROVAL_CONFIG = "tool_approval_config";

const { data: effectiveSettings } = useQuery({
  key: () => ["bot-settings", botIdRef.value],
  query: async () => {
    const response = await connectClients.settings.getBotSettings({ botId: botIdRef.value });
    return response.settings;
  },
  enabled: () => !!botIdRef.value,
});

const settings = computed(() => effectiveSettings.value?.settings);

const { mutateAsync: updateSettings, isLoading: saveLoading } = useMutation({
  mutation: async (body: Partial<BotSettings> & { toolApprovalConfig?: ToolApprovalConfig }) => {
    const response = await connectClients.settings.updateBotSettings({
      botId: botIdRef.value,
      settings: body,
    });
    return response.settings?.settings;
  },
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-settings", botIdRef.value] }),
});

const { mutateAsync: restoreInheritance, isLoading: restoreLoading } = useMutation({
  mutation: async (fields: string[]) => {
    const response = await connectClients.settings.restoreBotSettingsInheritance({
      botId: botIdRef.value,
      fields,
    });
    return response.settings;
  },
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-settings", botIdRef.value] }),
});

const form = reactive<{ tool_approval_config: ToolApprovalConfig }>({
  tool_approval_config: defaultToolApprovalConfig(),
});

function normalizeToolApprovalConfig(raw: unknown): ToolApprovalConfig {
  const defaults = defaultToolApprovalConfig();
  if (!raw || typeof raw !== "object") return defaults;
  const value = raw as Partial<ToolApprovalConfig>;
  return {
    enabled: value.enabled ?? defaults.enabled,
    write: {
      require_approval: value.write?.require_approval ?? defaults.write.require_approval,
      bypass_globs: value.write?.bypass_globs ?? defaults.write.bypass_globs,
      force_review_globs: value.write?.force_review_globs ?? defaults.write.force_review_globs,
    },
    edit: {
      require_approval: value.edit?.require_approval ?? defaults.edit.require_approval,
      bypass_globs: value.edit?.bypass_globs ?? defaults.edit.bypass_globs,
      force_review_globs: value.edit?.force_review_globs ?? defaults.edit.force_review_globs,
    },
    exec: {
      require_approval: value.exec?.require_approval ?? defaults.exec.require_approval,
      bypass_commands: value.exec?.bypass_commands ?? defaults.exec.bypass_commands,
      force_review_commands:
        value.exec?.force_review_commands ?? defaults.exec.force_review_commands,
    },
  };
}

function toolApprovalPolicy(tool: ApprovalTool) {
  return form.tool_approval_config[tool];
}

function bypassList(tool: ApprovalTool): string[] {
  const policy = toolApprovalPolicy(tool);
  return tool === "exec"
    ? (policy as ToolApprovalExecPolicy).bypass_commands
    : (policy as ToolApprovalFilePolicy).bypass_globs;
}

function forceReviewList(tool: ApprovalTool): string[] {
  const policy = toolApprovalPolicy(tool);
  return tool === "exec"
    ? (policy as ToolApprovalExecPolicy).force_review_commands
    : (policy as ToolApprovalFilePolicy).force_review_globs;
}

function approvalBypassText(tool: ApprovalTool): string {
  return bypassList(tool).join("\n");
}

function approvalForceReviewText(tool: ApprovalTool): string {
  return forceReviewList(tool).join("\n");
}

function updateApprovalBypass(tool: ApprovalTool, raw: string) {
  const values = raw
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean);
  if (tool === "exec") {
    form.tool_approval_config.exec.bypass_commands = values;
  } else {
    form.tool_approval_config[tool].bypass_globs = values;
  }
}

function updateApprovalForceReview(tool: ApprovalTool, raw: string) {
  const values = raw
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean);
  if (tool === "exec") {
    form.tool_approval_config.exec.force_review_commands = values;
  } else {
    form.tool_approval_config[tool].force_review_globs = values;
  }
}

function bypassPlaceholder(tool: ApprovalTool): string {
  return tool === "exec"
    ? t("bots.toolApproval.placeholders.execBypass")
    : t("bots.toolApproval.placeholders.fileBypass");
}

function forceReviewPlaceholder(tool: ApprovalTool): string {
  return tool === "exec"
    ? t("bots.toolApproval.placeholders.execMustReview")
    : t("bots.toolApproval.placeholders.fileMustReview");
}

watch(
  settings,
  (val) => {
    if (val) {
      form.tool_approval_config = normalizeToolApprovalConfig(
        (val as BotSettings & { toolApprovalConfig?: unknown }).toolApprovalConfig,
      );
    }
  },
  { immediate: true },
);

const hasChanges = computed(() => {
  if (!settings.value) return false;
  const current = normalizeToolApprovalConfig(
    (settings.value as BotSettings & { toolApprovalConfig?: unknown }).toolApprovalConfig,
  );
  return JSON.stringify(form.tool_approval_config) !== JSON.stringify(current);
});

async function handleSave() {
  try {
    await updateSettings({ toolApprovalConfig: form.tool_approval_config });
    toast.success(t("bots.settings.saveSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("common.saveFailed")));
  }
}

async function handleRestoreInheritance(fields: string[]) {
  try {
    await restoreInheritance(fields);
    toast.success(t("bots.settings.inheritance.restoreSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("bots.settings.inheritance.restoreFailed")));
  }
}
</script>
