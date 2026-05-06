<template>
  <div class="max-w-3xl mx-auto space-y-6">
    <div class="space-y-1">
      <h2 class="text-sm font-semibold text-foreground">
        {{ $t("bots.access.title") }}
      </h2>
      <p class="text-xs text-muted-foreground">
        {{ $t("bots.access.subtitle") }}
      </p>
    </div>

    <!-- Default Effect -->
    <section class="rounded-lg border border-border bg-card p-4 space-y-3">
      <div>
        <p class="text-xs font-medium text-foreground">
          {{ $t("bots.access.defaultEffectTitle") }}
        </p>
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.access.defaultEffectDescription") }}
        </p>
      </div>
      <div class="flex items-center gap-3">
        <button
          class="flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            defaultEffectDraft === 'deny'
              ? 'border-destructive bg-destructive/10 text-destructive'
              : 'border-border bg-card text-muted-foreground hover:bg-accent'
          "
          :disabled="isSavingDefaultEffect"
          @click="defaultEffectDraft = 'deny'"
        >
          <span
            class="size-2 rounded-full"
            :class="defaultEffectDraft === 'deny' ? 'bg-destructive' : 'bg-muted-foreground/40'"
          />
          {{ $t("bots.access.effectDeny") }}
        </button>
        <button
          class="flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            defaultEffectDraft === 'allow'
              ? 'border-emerald-500 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
              : 'border-border bg-card text-muted-foreground hover:bg-accent'
          "
          :disabled="isSavingDefaultEffect"
          @click="defaultEffectDraft = 'allow'"
        >
          <span
            class="size-2 rounded-full"
            :class="defaultEffectDraft === 'allow' ? 'bg-emerald-500' : 'bg-muted-foreground/40'"
          />
          {{ $t("bots.access.effectAllow") }}
        </button>
        <Button
          size="sm"
          :disabled="!hasDefaultEffectChanges || isSavingDefaultEffect"
          @click="handleSaveDefaultEffect"
        >
          <Spinner v-if="isSavingDefaultEffect" class="mr-1.5" />
          {{ $t("common.save") }}
        </Button>
      </div>
    </section>

    <!-- Rules -->
    <section class="rounded-lg border border-border bg-card p-4 [&>:nth-of-type(n+2)]:mt-4">
      <div class="flex items-center justify-between">
        <div>
          <h3 class="text-sm font-semibold text-foreground">
            {{ $t("bots.access.rulesTitle") }}
          </h3>
          <p class="text-xs text-muted-foreground">
            {{ $t("bots.access.rulesDescription") }}
          </p>
        </div>
        <Button v-if="!formVisible" size="sm" @click="openAddDialog">
          <Plus class="mr-1.5 size-3.5" />
          {{ $t("bots.access.addRule") }}
        </Button>
      </div>
      <div v-if="isLoadingRules" class="flex justify-center py-8">
        <Spinner />
      </div>
      <Empty
        v-else-if="rules.length === 0"
        :title="$t('bots.access.rulesEmpty')"
        class="empty:hidden"
        :description="$t('bots.access.rulesEmptyDescription')"
      />
      <div
        v-else
        ref="sortableListRef"
        class="space-y-2"
        :class="{ 'pointer-events-none opacity-60': isReordering }"
      >
        <div
          v-for="rule in draggableRules"
          :key="rule.id"
          class="flex items-center gap-3 rounded-md border border-border bg-background px-3 py-2.5"
        >
          <button
            type="button"
            class="acl-drag-handle shrink-0 cursor-grab touch-none rounded p-1 text-muted-foreground hover:bg-accent active:cursor-grabbing"
            :aria-label="$t('bots.access.dragToReorder')"
            :disabled="isReordering"
          >
            <GripVertical class="size-3.5" />
          </button>

          <!-- Priority badge -->
          <span
            class="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs font-mono text-muted-foreground"
          >
            {{ rule.priority }}
          </span>

          <!-- Effect badge -->
          <span
            class="shrink-0 rounded px-1.5 py-0.5 text-xs font-medium"
            :class="
              rule.effect === 'allow'
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                : 'bg-destructive/10 text-destructive'
            "
          >
            {{
              rule.effect === "allow" ? $t("bots.access.effectAllow") : $t("bots.access.effectDeny")
            }}
          </span>

          <!-- Subject + scope -->
          <div class="min-w-0 flex-1 space-y-0.5">
            <p class="truncate text-xs text-foreground">
              {{ describeSubject(rule) }}
            </p>
            <p v-if="rule.source_scope" class="truncate text-xs text-muted-foreground">
              {{ describeScope(rule.source_scope) }}
            </p>
            <p v-if="rule.description" class="truncate text-xs text-muted-foreground italic">
              {{ rule.description }}
            </p>
          </div>

          <!-- Enabled toggle -->
          <Switch
            :model-value="rule.enabled"
            class="shrink-0"
            @update:model-value="(v) => handleToggleEnabled(rule, !!v)"
          />

          <!-- Actions -->
          <div class="shrink-0 flex items-center gap-1">
            <Button variant="ghost" size="icon-sm" @click="openEditDialog(rule)">
              <SquarePen class="size-3.5" />
            </Button>
            <ConfirmPopover
              :title="$t('bots.access.deleteConfirmTitle')"
              :description="$t('bots.access.deleteConfirmDescription')"
              :confirm-label="$t('common.delete')"
              @confirm="handleDeleteRule(rule.id!)"
            >
              <Button
                variant="ghost"
                size="icon-sm"
                class="text-destructive hover:text-destructive"
              >
                <Trash2 class="size-3.5" />
              </Button>
            </ConfirmPopover>
          </div>
        </div>
      </div>
    </section>

    <!-- Inline Add/Edit Rule Form -->
    <section v-if="formVisible" class="rounded-lg border border-border bg-card p-4 space-y-4">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-semibold text-foreground">
          {{ editingRule ? $t("bots.access.editRule") : $t("bots.access.addRule") }}
        </h3>
        <Button variant="ghost" size="icon-sm" class="size-7" @click="formVisible = false">
          <X class="size-4" />
        </Button>
      </div>

      <form class="space-y-4" @submit.prevent="handleSaveRule">
        <div class="space-y-1.5">
          <Label>{{ $t("bots.access.enabled") }}</Label>
          <div class="flex items-center gap-2 h-9">
            <Switch v-model="ruleForm.enabled" />
            <span class="text-xs text-muted-foreground">{{
              ruleForm.enabled ? $t("common.yes") : $t("common.no")
            }}</span>
          </div>
        </div>

        <!-- Effect -->
        <div class="space-y-1.5">
          <Label>{{ $t("bots.access.effect") }}</Label>
          <div class="flex gap-2">
            <button
              type="button"
              class="flex-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors"
              :class="
                ruleForm.effect === 'deny'
                  ? 'border-destructive bg-destructive/10 text-destructive'
                  : 'border-border text-muted-foreground hover:bg-accent'
              "
              @click="ruleForm.effect = 'deny'"
            >
              {{ $t("bots.access.effectDeny") }}
            </button>
            <button
              type="button"
              class="flex-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors"
              :class="
                ruleForm.effect === 'allow'
                  ? 'border-emerald-500 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                  : 'border-border text-muted-foreground hover:bg-accent'
              "
              @click="ruleForm.effect = 'allow'"
            >
              {{ $t("bots.access.effectAllow") }}
            </button>
          </div>
        </div>

        <!-- Subject Kind -->
        <div class="space-y-1.5">
          <Label>{{ $t("bots.access.subjectKind") }}</Label>
          <div class="grid grid-cols-3 gap-2">
            <button
              v-for="kind in subjectKinds"
              :key="kind.value"
              type="button"
              class="rounded-md border px-2 py-1.5 text-xs font-medium transition-colors text-center"
              :class="
                ruleForm.subjectKind === kind.value
                  ? 'border-primary bg-primary/10 text-primary'
                  : 'border-border text-muted-foreground hover:bg-accent'
              "
              @click="handleSubjectKindChange(kind.value)"
            >
              {{ kind.label }}
            </button>
          </div>
        </div>

        <!-- Channel Type (when subjectKind === 'channel_type') -->
        <div v-if="ruleForm.subjectKind === 'channel_type'" class="space-y-1.5">
          <Label>{{ $t("bots.access.channelType") }}</Label>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="ch in commonChannels"
              :key="ch"
              type="button"
              class="rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors"
              :class="
                ruleForm.subjectChannelType === ch
                  ? 'border-primary bg-primary/10 text-primary'
                  : 'border-border text-muted-foreground hover:bg-accent'
              "
              @click="ruleForm.subjectChannelType = ch"
            >
              {{ ch }}
            </button>
          </div>
          <Input
            v-model="ruleForm.subjectChannelType"
            :placeholder="$t('bots.access.channelTypePlaceholder')"
            class="mt-1.5"
          />
        </div>

        <!-- Channel Identity (when subjectKind === 'channel_identity') -->
        <div v-if="ruleForm.subjectKind === 'channel_identity'" class="space-y-1.5">
          <Label>{{ $t("bots.access.identitySelector") }}</Label>
          <SearchableSelectPopover
            v-model="ruleForm.channelIdentityId"
            :options="identityOptions"
            :placeholder="$t('bots.access.selectIdentity')"
            :aria-label="$t('bots.access.selectIdentity')"
            :search-placeholder="$t('bots.access.searchIdentity')"
            :search-aria-label="$t('bots.access.searchIdentity')"
            :empty-text="$t('bots.access.noIdentityCandidates')"
          >
            <template #option-label="{ option }">
              <div class="flex min-w-0 items-center gap-2 text-left">
                <Avatar class="size-6 shrink-0">
                  <AvatarImage :src="option.meta?.avatarUrl" :alt="option.label" />
                  <AvatarFallback>{{ option.label.slice(0, 2).toUpperCase() }}</AvatarFallback>
                </Avatar>
                <div class="min-w-0">
                  <div class="truncate text-xs">
                    {{ option.label }}
                  </div>
                  <div
                    v-if="option.meta?.linkedUsername"
                    class="truncate text-xs text-muted-foreground"
                  >
                    @{{ option.meta.linkedUsername }}
                  </div>
                </div>
              </div>
            </template>
          </SearchableSelectPopover>
        </div>

        <!-- Source Scope -->
        <details
          class="group"
          :open="scopeOpen"
          @toggle="scopeOpen = ($event.target as HTMLDetailsElement).open"
        >
          <summary
            class="flex cursor-pointer items-center gap-1 text-xs font-medium text-foreground select-none list-none"
          >
            <ChevronRight class="size-3 transition-transform group-open:rotate-90" />
            {{ $t("bots.access.sourceScopeTitle") }}
          </summary>
          <div class="mt-3 space-y-3 pl-4 border-l border-border">
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.access.sourceScopeDescription") }}
            </p>

            <!-- Conversation Type -->
            <div class="space-y-1.5">
              <Label>{{ $t("bots.access.conversationType") }}</Label>
              <div class="flex gap-2">
                <button
                  v-for="ct in conversationTypes"
                  :key="ct.value"
                  type="button"
                  class="rounded-md border px-2.5 py-1 text-xs font-medium transition-colors"
                  :class="
                    ruleForm.sourceConversationType === ct.value
                      ? 'border-primary bg-primary/10 text-primary'
                      : 'border-border text-muted-foreground hover:bg-accent'
                  "
                  @click="
                    ruleForm.sourceConversationType =
                      ruleForm.sourceConversationType === ct.value ? '' : ct.value
                  "
                >
                  {{ ct.label }}
                </button>
              </div>
            </div>

            <!-- Searchable conversation (identity or platform type) -->
            <div v-if="showConversationSearch" class="space-y-1.5">
              <Label>{{ $t("bots.access.conversationSource") }}</Label>
              <p class="text-xs text-muted-foreground">
                {{ $t("bots.access.conversationSourceDescription") }}
              </p>
              <SearchableSelectPopover
                v-model="ruleForm.observedConversationRouteId"
                :options="observedConversationOptions"
                :show-group-headers="false"
                :placeholder="$t('bots.access.selectConversationSource')"
                :aria-label="$t('bots.access.selectConversationSource')"
                :search-placeholder="$t('bots.access.searchConversationSource')"
                :search-aria-label="$t('bots.access.searchConversationSource')"
                :empty-text="observedConversationEmptyText"
                @update:model-value="onConversationSourceChange"
              >
                <template #option-label="{ option }">
                  <div class="min-w-0 flex-1 text-left">
                    <div class="truncate text-xs">
                      {{ option.label }}
                    </div>
                    <div class="truncate text-xs text-muted-foreground">
                      {{
                        buildConversationStableId(
                          option.meta as AclObservedConversationCandidate | undefined,
                        )
                      }}
                    </div>
                  </div>
                </template>
              </SearchableSelectPopover>

              <details class="group/conversation-manual">
                <summary
                  class="cursor-pointer list-none text-xs font-medium text-muted-foreground hover:text-foreground select-none"
                >
                  <ChevronRight
                    class="mr-0.5 inline size-3 transition-transform group-open/conversation-manual:rotate-90"
                  />
                  {{ $t("bots.access.manualConversationIds") }}
                </summary>
                <p class="mt-2 text-xs text-muted-foreground">
                  {{ $t("bots.access.manualConversationIdsHint") }}
                </p>
                <div class="mt-2 space-y-3 pl-1">
                  <div class="space-y-1.5">
                    <Label>{{ $t("bots.access.conversationId") }}</Label>
                    <Input
                      v-model="ruleForm.sourceConversationId"
                      :placeholder="$t('bots.access.conversationIdPlaceholder')"
                    />
                  </div>
                  <div class="space-y-1.5">
                    <Label>{{ $t("bots.access.threadId") }}</Label>
                    <Input
                      v-model="ruleForm.sourceThreadId"
                      :placeholder="$t('bots.access.threadIdPlaceholder')"
                    />
                  </div>
                </div>
              </details>
            </div>

            <!-- No identity: manual IDs only (no search API) -->
            <template v-else>
              <p
                v-if="ruleForm.subjectKind === 'channel_identity'"
                class="text-xs text-muted-foreground"
              >
                {{ $t("bots.access.pickIdentityForConversationSearch") }}
              </p>
              <p
                v-else-if="ruleForm.subjectKind === 'channel_type'"
                class="text-xs text-muted-foreground"
              >
                {{ $t("bots.access.pickChannelTypeForConversationSearch") }}
              </p>
              <div class="space-y-1.5">
                <Label>{{ $t("bots.access.conversationId") }}</Label>
                <Input
                  v-model="ruleForm.sourceConversationId"
                  :placeholder="$t('bots.access.conversationIdPlaceholder')"
                />
                <p class="text-xs text-muted-foreground">
                  {{ $t("bots.access.conversationIdManualHint") }}
                </p>
              </div>
              <div class="space-y-1.5">
                <Label>{{ $t("bots.access.threadId") }}</Label>
                <Input
                  v-model="ruleForm.sourceThreadId"
                  :placeholder="$t('bots.access.threadIdPlaceholder')"
                />
              </div>
            </template>

            <Button type="button" variant="ghost" size="sm" @click="clearScopeFields">
              {{ $t("bots.access.clearScope") }}
            </Button>
          </div>
        </details>

        <!-- Description -->
        <div class="space-y-1.5">
          <Label>{{ $t("bots.access.description") }}</Label>
          <Input
            v-model="ruleForm.description"
            :placeholder="$t('bots.access.descriptionPlaceholder')"
          />
        </div>

        <p v-if="formError" class="text-xs text-destructive">
          {{ formError }}
        </p>

        <div class="flex justify-end gap-2 pt-2">
          <Button type="button" variant="outline" size="sm" @click="formVisible = false">
            {{ $t("common.cancel") }}
          </Button>
          <Button type="submit" size="sm" :disabled="isSavingRule">
            <Spinner v-if="isSavingRule" class="mr-1.5" />
            {{ $t("common.save") }}
          </Button>
        </div>
      </form>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useSortable } from "@vueuse/integrations/useSortable";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
import { useQuery, useQueryCache } from "@pinia/colada";
import { Plus, GripVertical, SquarePen, Trash2, ChevronRight, X } from "lucide-vue-next";
import {
  Button,
  Input,
  Label,
  Switch,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Spinner,
  Empty,
} from "@stringke/ui";
import ConfirmPopover from "@/components/confirm-popover/index.vue";
import SearchableSelectPopover from "@/components/searchable-select-popover/index.vue";
import { connectClients } from "@/lib/connect-client";
import { recordValue } from "@/lib/connect-runtime";
import { resolveApiErrorMessage } from "@/utils/api-error";
import { formatRelativeTime } from "@/utils/date-time";

interface AclSourceScope {
  channel?: string;
  conversation_type?: string;
  conversation_id?: string;
  thread_id?: string;
}

interface AclRule {
  id?: string;
  priority?: number;
  enabled?: boolean;
  effect?: string;
  subject_kind?: string;
  subject_channel_type?: string;
  channel_identity_id?: string;
  channel_identity_display_name?: string;
  channel_subject_id?: string;
  channel_type?: string;
  source_scope?: AclSourceScope;
  description?: string;
}

interface AclObservedConversationCandidate {
  route_id?: string;
  conversation_name?: string;
  conversation_id?: string;
  thread_id?: string;
  channel?: string;
  conversation_type?: string;
  last_observed_at?: string;
}

interface AclIdentityCandidate {
  id?: string;
  channel?: string;
  display_name?: string;
  channel_subject_id?: string;
  avatar_url?: string;
  linked_username?: string;
}

// ---- props ----

const props = defineProps<{
  botId: string;
}>();

const { t } = useI18n();
const queryCache = useQueryCache();

function stringFromRecord(value: Record<string, unknown>, key: string): string {
  const raw = value[key];
  return typeof raw === "string" ? raw : "";
}

function boolFromRecord(value: Record<string, unknown>, key: string, fallback: boolean): boolean {
  const raw = value[key];
  return typeof raw === "boolean" ? raw : fallback;
}

function sourceScopeFromValue(value: unknown): AclSourceScope | undefined {
  const record = recordValue(value);
  const scope: AclSourceScope = {
    channel: stringFromRecord(record, "channel"),
    conversation_type: stringFromRecord(record, "conversation_type"),
    conversation_id: stringFromRecord(record, "conversation_id"),
    thread_id: stringFromRecord(record, "thread_id"),
  };
  return Object.values(scope).some(Boolean) ? scope : undefined;
}

function aclRuleFromConnect(
  rule: Awaited<ReturnType<typeof connectClients.acl.listAclRules>>["rules"][number],
): AclRule {
  const metadata = recordValue(rule.metadata);
  const subjectKind =
    stringFromRecord(metadata, "subject_kind") ||
    (rule.identityId ? "channel_identity" : rule.channel ? "channel_type" : "all");
  return {
    id: rule.id,
    priority: rule.priority,
    enabled: boolFromRecord(metadata, "enabled", true),
    effect: rule.effect || stringFromRecord(metadata, "effect") || "deny",
    subject_kind: subjectKind,
    subject_channel_type: stringFromRecord(metadata, "subject_channel_type") || rule.channel,
    channel_identity_id: stringFromRecord(metadata, "channel_identity_id") || rule.identityId,
    channel_identity_display_name: stringFromRecord(metadata, "channel_identity_display_name"),
    channel_subject_id: stringFromRecord(metadata, "channel_subject_id") || rule.identityId,
    channel_type: stringFromRecord(metadata, "channel_type") || rule.channel,
    source_scope: sourceScopeFromValue(metadata.source_scope),
    description: stringFromRecord(metadata, "description"),
  };
}

function identityFromConnect(
  identity: Awaited<
    ReturnType<typeof connectClients.acl.listAclChannelIdentities>
  >["identities"][number],
): AclIdentityCandidate {
  const metadata = recordValue(identity.metadata);
  return {
    id: identity.id,
    channel: identity.channel,
    display_name: identity.displayName,
    channel_subject_id: stringFromRecord(metadata, "channel_subject_id") || identity.id,
    avatar_url: stringFromRecord(metadata, "avatar_url"),
    linked_username: stringFromRecord(metadata, "linked_username"),
  };
}

function observedConversationFromConnect(
  conversation: Awaited<
    ReturnType<typeof connectClients.acl.listAclObservedConversations>
  >["conversations"][number],
): AclObservedConversationCandidate {
  const metadata = recordValue(conversation.metadata);
  return {
    route_id: conversation.id,
    conversation_name: conversation.title,
    conversation_id: stringFromRecord(metadata, "conversation_id") || conversation.id,
    thread_id: stringFromRecord(metadata, "thread_id"),
    channel: conversation.channel,
    conversation_type: stringFromRecord(metadata, "conversation_type"),
    last_observed_at: stringFromRecord(metadata, "last_observed_at"),
  };
}

function ruleMetadataFromForm(): Record<string, unknown> {
  return {
    priority: editingRule.value?.id ? (editingRule.value.priority ?? 0) : nextRulePriority(),
    enabled: ruleForm.enabled,
    subject_kind: ruleForm.subjectKind,
    subject_channel_type:
      ruleForm.subjectKind === "channel_type"
        ? ruleForm.subjectChannelType || undefined
        : undefined,
    channel_identity_id:
      ruleForm.subjectKind === "channel_identity"
        ? ruleForm.channelIdentityId || undefined
        : undefined,
    source_scope: buildSourceScope(),
    description: ruleForm.description || undefined,
  };
}

// ---- constants ----

const commonChannels = ["discord", "feishu", "qq", "telegram", "wecom", "local"];

const subjectKinds = computed(() => [
  { value: "all", label: t("bots.access.subjectAll") },
  { value: "channel_type", label: t("bots.access.subjectChannelType") },
  { value: "channel_identity", label: t("bots.access.subjectChannelIdentity") },
]);

const conversationTypes = computed(() => [
  { value: "private", label: t("bots.access.privateConversationType") },
  { value: "group", label: t("bots.access.groupConversationType") },
  { value: "thread", label: t("bots.access.threadConversationType") },
]);

// ---- queries ----

const { data: rulesData, isLoading: isLoadingRules } = useQuery({
  key: () => ["bot-acl-rules", props.botId],
  query: async () => {
    const response = await connectClients.acl.listAclRules({ botId: props.botId });
    return response.rules.map(aclRuleFromConnect);
  },
  enabled: () => !!props.botId,
});

const { data: defaultEffectData } = useQuery({
  key: () => ["bot-acl-default-effect", props.botId],
  query: async () => {
    return await connectClients.acl.getAclDefaultEffect({ botId: props.botId });
  },
  enabled: () => !!props.botId,
});

const { data: identityCandidates } = useQuery({
  key: () => ["bot-acl-identities", props.botId],
  query: async () => {
    const response = await connectClients.acl.listAclChannelIdentities({
      botId: props.botId,
      channel: "",
      page: { pageSize: 100, pageToken: "" },
    });
    return response.identities.map(identityFromConnect);
  },
  enabled: () => !!props.botId,
});

interface RuleForm {
  enabled: boolean;
  effect: string;
  subjectKind: string;
  subjectChannelType: string;
  channelIdentityId: string;
  observedConversationRouteId: string;
  sourceConversationType: string;
  sourceConversationId: string;
  sourceThreadId: string;
  description: string;
}

function createRuleForm(): RuleForm {
  return {
    enabled: true,
    effect: "deny",
    subjectKind: "all",
    subjectChannelType: "",
    channelIdentityId: "",
    observedConversationRouteId: "",
    sourceConversationType: "",
    sourceConversationId: "",
    sourceThreadId: "",
    description: "",
  };
}

const ruleForm = reactive(createRuleForm());

const dialogIdentityId = computed(() =>
  ruleForm.subjectKind === "channel_identity" ? ruleForm.channelIdentityId.trim() : "",
);

const dialogChannelTypeTrimmed = computed(() =>
  ruleForm.subjectKind === "channel_type" ? ruleForm.subjectChannelType.trim() : "",
);

const { data: observedByIdentityData, isLoading: isLoadingObservedIdentity } = useQuery({
  key: () => ["bot-acl-observed", props.botId, dialogIdentityId.value],
  query: async () => {
    const response = await connectClients.acl.listAclObservedConversations({
      botId: props.botId,
      identityId: dialogIdentityId.value,
      page: { pageSize: 100, pageToken: "" },
    });
    return response.conversations.map(observedConversationFromConnect);
  },
  enabled: () => !!props.botId && !!dialogIdentityId.value,
});

const { data: observedByChannelTypeData, isLoading: isLoadingObservedChannelType } = useQuery({
  key: () => ["bot-acl-observed-channel-type", props.botId, dialogChannelTypeTrimmed.value],
  query: async () => {
    const response = await connectClients.acl.listAclObservedConversationsByChannelType({
      botId: props.botId,
      channel: dialogChannelTypeTrimmed.value,
      page: { pageSize: 100, pageToken: "" },
    });
    return response.conversations.map(observedConversationFromConnect);
  },
  enabled: () => !!props.botId && !!dialogChannelTypeTrimmed.value,
});

/** Active observed-conversation list for the current subject (identity or platform type). */
const observedConversationsForRule = computed(() => {
  if (ruleForm.subjectKind === "channel_identity" && dialogIdentityId.value) {
    return observedByIdentityData.value;
  }
  if (ruleForm.subjectKind === "channel_type" && dialogChannelTypeTrimmed.value) {
    return observedByChannelTypeData.value;
  }
  return undefined;
});

const showConversationSearch = computed(
  () =>
    (ruleForm.subjectKind === "channel_identity" && !!dialogIdentityId.value) ||
    (ruleForm.subjectKind === "channel_type" && !!dialogChannelTypeTrimmed.value),
);

const observedConversationEmptyText = computed(() => {
  if (
    ruleForm.subjectKind === "channel_identity" &&
    dialogIdentityId.value &&
    isLoadingObservedIdentity.value
  ) {
    return t("common.loading");
  }
  if (
    ruleForm.subjectKind === "channel_type" &&
    dialogChannelTypeTrimmed.value &&
    isLoadingObservedChannelType.value
  ) {
    return t("common.loading");
  }
  return t("bots.access.noObservedConversations");
});

// ---- derived ----

const rules = computed(() => rulesData.value ?? []);
const identities = computed(() => identityCandidates.value ?? []);

const draggableRules = ref<AclRule[]>([]);
const sortableListRef = ref<HTMLElement | null>(null);
const isReordering = ref(false);

watch(
  rules,
  (r) => {
    draggableRules.value = [...r];
  },
  { immediate: true },
);

function nextRulePriority(): number {
  const last = rules.value.at(-1);
  return (last?.priority ?? -10) + 10;
}

useSortable(sortableListRef, draggableRules, {
  animation: 150,
  handle: ".acl-drag-handle",
  watchElement: true,
  onEnd: (evt) => {
    void handleRulesReorderEnd(evt);
  },
});

async function handleRulesReorderEnd(evt: { oldIndex?: number; newIndex?: number }) {
  if (evt.oldIndex === undefined || evt.newIndex === undefined || evt.oldIndex === evt.newIndex) {
    return;
  }
  // Compute the new order from indices directly — don't rely on draggableRules
  // being updated yet, because useSortable syncs the array asynchronously after onEnd.
  const reordered = [...draggableRules.value];
  const [moved] = reordered.splice(evt.oldIndex, 1);
  reordered.splice(evt.newIndex, 0, moved);
  draggableRules.value = reordered;

  isReordering.value = true;
  try {
    await connectClients.acl.reorderAclRules({
      botId: props.botId,
      ruleIds: reordered.map((rule) => rule.id ?? "").filter(Boolean),
    });
    queryCache.invalidateQueries({ key: ["bot-acl-rules", props.botId] });
    toast.success(t("bots.access.rulesReordered"));
  } catch (e) {
    draggableRules.value = [...(rules.value ?? [])];
    toast.error(resolveApiErrorMessage(e, t("bots.access.reorderFailed")));
  } finally {
    isReordering.value = false;
  }
}

const identityOptions = computed(() =>
  identities.value.map((i) => ({
    value: i.id ?? "",
    label: i.display_name || i.channel_subject_id || i.id || "",
    meta: {
      avatarUrl: i.avatar_url,
      linkedUsername: i.linked_username,
    },
  })),
);

const observedConversationOptions = computed(() =>
  (observedConversationsForRule.value ?? []).map((c) => {
    const label = buildConversationLabel(c);
    const keywords = [
      c.conversation_name,
      c.conversation_id,
      c.thread_id,
      c.channel,
      c.conversation_type,
    ].filter((x): x is string => Boolean(x && String(x).trim()));
    return {
      value: c.route_id ?? "",
      label,
      description: c.last_observed_at ? formatRelativeTime(c.last_observed_at) : undefined,
      keywords,
      meta: c,
    };
  }),
);

/** Primary display label: name when available, stable ID otherwise. */
function buildConversationLabel(c: AclObservedConversationCandidate | undefined): string {
  if (!c) return "";
  const name = c.conversation_name?.trim();
  if (name) return name;
  return c.conversation_id || c.route_id || "";
}

/** Subtitle always shows the stable platform identifiers for verification. */
function buildConversationStableId(c: AclObservedConversationCandidate | undefined): string {
  if (!c) return "";
  const parts: string[] = [];
  if (c.channel) parts.push(c.channel);
  if (c.conversation_type) parts.push(c.conversation_type);
  if (c.conversation_id) parts.push(c.conversation_id);
  if (c.thread_id) parts.push(`thread:${c.thread_id}`);
  return parts.join(" · ");
}

function onConversationSourceChange(routeId: string) {
  ruleForm.observedConversationRouteId = routeId;
  if (!routeId.trim()) {
    ruleForm.sourceConversationType = "";
    ruleForm.sourceConversationId = "";
    ruleForm.sourceThreadId = "";
    return;
  }
  applyObservedConversation(routeId);
}

// ---- default effect ----

const defaultEffectDraft = ref("allow");
const isSavingDefaultEffect = ref(false);

watch(
  defaultEffectData,
  (data) => {
    if (data?.effect) {
      defaultEffectDraft.value = data.effect;
    }
  },
  { immediate: true },
);

const hasDefaultEffectChanges = computed(
  () => defaultEffectDraft.value !== (defaultEffectData.value?.effect ?? "allow"),
);

async function handleSaveDefaultEffect() {
  isSavingDefaultEffect.value = true;
  try {
    await connectClients.acl.setAclDefaultEffect({
      botId: props.botId,
      effect: defaultEffectDraft.value,
    });
    queryCache.invalidateQueries({ key: ["bot-acl-default-effect", props.botId] });
    toast.success(t("bots.access.defaultEffectSaved"));
  } catch (e) {
    toast.error(resolveApiErrorMessage(e, t("bots.access.saveFailed")));
  } finally {
    isSavingDefaultEffect.value = false;
  }
}

// ---- rule form ----

const formVisible = ref(false);
const editingRule = ref<AclRule | null>(null);
const formError = ref("");
const isSavingRule = ref(false);

watch(
  () =>
    [
      formVisible.value,
      ruleForm.subjectKind,
      dialogIdentityId.value,
      dialogChannelTypeTrimmed.value,
      ruleForm.sourceConversationType,
      ruleForm.sourceConversationId,
      ruleForm.sourceThreadId,
      observedByIdentityData.value,
      observedByChannelTypeData.value,
    ] as const,
  () => {
    if (!formVisible.value) return;
    const hasIdentity = ruleForm.subjectKind === "channel_identity" && !!dialogIdentityId.value;
    const hasChannelType =
      ruleForm.subjectKind === "channel_type" && !!dialogChannelTypeTrimmed.value;
    if (!hasIdentity && !hasChannelType) return;
    const items = hasIdentity
      ? (observedByIdentityData.value ?? [])
      : (observedByChannelTypeData.value ?? []);
    const match = items.find(
      (c) =>
        (c.conversation_type ?? "") === (ruleForm.sourceConversationType ?? "") &&
        (c.conversation_id ?? "") === (ruleForm.sourceConversationId ?? "") &&
        (c.thread_id ?? "") === (ruleForm.sourceThreadId ?? ""),
    );
    const nextRoute = match?.route_id ?? "";
    if (nextRoute !== ruleForm.observedConversationRouteId) {
      ruleForm.observedConversationRouteId = nextRoute;
    }
  },
);

watch(
  () => ruleForm.channelIdentityId,
  (id, prev) => {
    if (!formVisible.value) return;
    if (prev !== undefined && prev !== "" && id !== prev) {
      ruleForm.observedConversationRouteId = "";
      ruleForm.sourceConversationType = "";
      ruleForm.sourceConversationId = "";
      ruleForm.sourceThreadId = "";
    }
  },
);

watch(
  () => ruleForm.subjectChannelType,
  (id, prev) => {
    if (!formVisible.value) return;
    if (ruleForm.subjectKind !== "channel_type") return;
    if (prev !== undefined && prev.trim() !== "" && id !== prev) {
      ruleForm.observedConversationRouteId = "";
      ruleForm.sourceConversationType = "";
      ruleForm.sourceConversationId = "";
      ruleForm.sourceThreadId = "";
    }
  },
);

const hasScopeValues = computed(
  () =>
    !!(ruleForm.sourceConversationType || ruleForm.sourceConversationId || ruleForm.sourceThreadId),
);

const scopeOpen = ref(false);

watch(hasScopeValues, (val) => {
  if (val) scopeOpen.value = true;
});

function openAddDialog() {
  editingRule.value = null;
  Object.assign(ruleForm, createRuleForm());
  scopeOpen.value = false;
  formError.value = "";
  formVisible.value = true;
}

function openEditDialog(rule: AclRule) {
  editingRule.value = rule;
  ruleForm.enabled = rule.enabled ?? true;
  ruleForm.effect = rule.effect ?? "deny";
  ruleForm.subjectKind = rule.subject_kind ?? "all";
  ruleForm.subjectChannelType = rule.subject_channel_type ?? "";
  ruleForm.channelIdentityId = rule.channel_identity_id ?? "";
  ruleForm.observedConversationRouteId = "";
  ruleForm.sourceConversationType = rule.source_scope?.conversation_type ?? "";
  ruleForm.sourceConversationId = rule.source_scope?.conversation_id ?? "";
  ruleForm.sourceThreadId = rule.source_scope?.thread_id ?? "";
  ruleForm.description = rule.description ?? "";
  scopeOpen.value = hasScopeValues.value;
  formError.value = "";
  formVisible.value = true;
}

function handleSubjectKindChange(kind: string) {
  ruleForm.subjectKind = kind;
  ruleForm.subjectChannelType = "";
  ruleForm.channelIdentityId = "";
  ruleForm.observedConversationRouteId = "";
  ruleForm.sourceConversationType = "";
  ruleForm.sourceConversationId = "";
  ruleForm.sourceThreadId = "";
}

function clearScopeFields() {
  ruleForm.observedConversationRouteId = "";
  ruleForm.sourceConversationType = "";
  ruleForm.sourceConversationId = "";
  ruleForm.sourceThreadId = "";
}

function applyObservedConversation(routeId: string) {
  const item = (observedConversationsForRule.value ?? []).find((c) => c.route_id === routeId);
  if (!item) return;
  ruleForm.sourceConversationType = item.conversation_type ?? "";
  ruleForm.sourceConversationId = item.conversation_id ?? "";
  ruleForm.sourceThreadId = item.thread_id ?? "";
}

function buildSourceScope(): AclSourceScope | undefined {
  const scope: AclSourceScope = {};
  if (ruleForm.sourceConversationType) scope.conversation_type = ruleForm.sourceConversationType;
  if (ruleForm.sourceConversationId) scope.conversation_id = ruleForm.sourceConversationId;
  if (ruleForm.sourceThreadId) scope.thread_id = ruleForm.sourceThreadId;
  if (!scope.conversation_type && !scope.conversation_id && !scope.thread_id) {
    return undefined;
  }
  return scope;
}

async function handleSaveRule() {
  formError.value = "";
  isSavingRule.value = true;
  try {
    const selectedIdentity = identities.value.find(
      (item) => item.id === ruleForm.channelIdentityId,
    );
    const channel =
      ruleForm.subjectKind === "channel_type"
        ? ruleForm.subjectChannelType
        : ruleForm.subjectKind === "channel_identity"
          ? (selectedIdentity?.channel ?? "")
          : "";
    const identityId =
      ruleForm.subjectKind === "channel_identity" ? ruleForm.channelIdentityId : "";
    const metadata = ruleMetadataFromForm();
    if (editingRule.value?.id) {
      await connectClients.acl.updateAclRule({
        id: editingRule.value.id,
        effect: ruleForm.effect,
        metadata,
      });
    } else {
      await connectClients.acl.createAclRule({
        botId: props.botId,
        channel,
        identityId,
        effect: ruleForm.effect,
        metadata,
      });
    }
    queryCache.invalidateQueries({ key: ["bot-acl-rules", props.botId] });
    toast.success(t("bots.access.ruleSaved"));
    formVisible.value = false;
  } catch (e) {
    formError.value = resolveApiErrorMessage(e, t("bots.access.saveFailed"));
  } finally {
    isSavingRule.value = false;
  }
}

async function handleDeleteRule(ruleId: string) {
  try {
    await connectClients.acl.deleteAclRule({ id: ruleId });
    queryCache.invalidateQueries({ key: ["bot-acl-rules", props.botId] });
    toast.success(t("bots.access.deleteSuccess"));
  } catch (e) {
    toast.error(resolveApiErrorMessage(e, t("bots.access.deleteFailed")));
  }
}

async function handleToggleEnabled(rule: AclRule, enabled: boolean) {
  try {
    await connectClients.acl.updateAclRule({
      id: rule.id!,
      effect: rule.effect ?? "deny",
      metadata: {
        priority: rule.priority ?? 0,
        enabled,
        subject_kind: rule.subject_kind ?? "all",
        channel_identity_id: rule.channel_identity_id,
        subject_channel_type: rule.subject_channel_type,
        source_scope: rule.source_scope,
        description: rule.description,
      },
    });
    queryCache.invalidateQueries({ key: ["bot-acl-rules", props.botId] });
  } catch (e) {
    toast.error(resolveApiErrorMessage(e, t("bots.access.saveFailed")));
  }
}

// ---- display helpers ----

function describeSubject(rule: AclRule): string {
  switch (rule.subject_kind) {
    case "all":
      return t("bots.access.subjectAllLabel");
    case "channel_type":
      return t("bots.access.subjectChannelTypeLabel", { channel: rule.subject_channel_type });
    case "channel_identity": {
      const display =
        rule.channel_identity_display_name ||
        rule.channel_subject_id ||
        rule.channel_identity_id ||
        "?";
      const platform = rule.channel_type ? ` (${rule.channel_type})` : "";
      return `${display}${platform}`;
    }
    default:
      return rule.subject_kind ?? "?";
  }
}

function describeScope(scope: AclSourceScope): string {
  const parts: string[] = [];
  if (scope.channel) parts.push(scope.channel);
  if (scope.conversation_type) parts.push(scope.conversation_type);
  if (scope.conversation_id) parts.push(scope.conversation_id);
  if (scope.thread_id) parts.push(`thread:${scope.thread_id}`);
  return parts.join(" › ");
}
</script>
