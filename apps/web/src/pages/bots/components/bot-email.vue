<template>
  <div class="max-w-4xl mx-auto space-y-5">
    <!-- Header -->
    <div class="flex items-start justify-between gap-3">
      <div class="space-y-1 min-w-0">
        <h3 class="text-sm font-semibold">
          {{ $t("bots.email.title") }}
        </h3>
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.email.subtitle") }}
        </p>
      </div>
    </div>

    <!-- Bindings section -->
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <h4 class="text-xs font-medium">
          {{ $t("bots.email.bindings") }}
        </h4>
        <Popover>
          <PopoverTrigger as-child>
            <Button size="sm" :disabled="!unboundProviders.length">
              <Plus class="mr-1.5" />
              {{ $t("bots.email.addBinding") }}
            </Button>
          </PopoverTrigger>
          <PopoverContent class="w-56 p-1" align="end">
            <button
              v-for="p in unboundProviders"
              :key="p.id"
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-3 py-2 text-xs transition-colors hover:bg-accent"
              :disabled="addingBinding"
              @click="handleAddBinding(p)"
            >
              <Spinner v-if="addingProviderId === p.id" class="size-3" />
              {{ p.name }}
              <span class="text-xs text-muted-foreground ml-auto">{{ p.type }}</span>
            </button>
          </PopoverContent>
        </Popover>
      </div>
      <div v-if="bindingsLoading" class="flex items-center gap-2 text-xs text-muted-foreground p-4">
        <Spinner />
        <span>{{ $t("common.loading") }}</span>
      </div>

      <div v-else-if="!bindings?.length" class="rounded-md border p-4">
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.email.noBindings") }}
        </p>
      </div>

      <div v-else class="space-y-2">
        <div v-for="binding in bindings" :key="binding.id" class="rounded-md border p-4">
          <div class="flex items-center justify-between">
            <div class="min-w-0">
              <p class="font-medium text-xs">
                {{ providerNameMap[binding.providerId] || binding.providerId }}
              </p>
              <p v-if="binding.address" class="text-xs text-muted-foreground mt-0.5">
                {{ binding.address }}
              </p>
            </div>
            <ConfirmPopover
              :message="$t('bots.email.unbindConfirm')"
              :loading="deletingId === binding.id"
              @confirm="handleDeleteBinding(binding.id!)"
            >
              <template #trigger>
                <Button variant="destructive" size="sm">
                  {{ $t("bots.email.unbind") }}
                </Button>
              </template>
            </ConfirmPopover>
          </div>
          <Separator class="my-3" />
          <div class="flex gap-6 text-xs">
            <label class="flex items-center gap-2 cursor-pointer">
              <Switch
                :model-value="binding.can_read"
                @update:model-value="(v) => handleTogglePerm(binding, 'can_read', !!v)"
              />
              <span>{{ $t("bots.email.canRead") }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <Switch
                :model-value="binding.can_write"
                @update:model-value="(v) => handleTogglePerm(binding, 'can_write', !!v)"
              />
              <span>{{ $t("bots.email.canWrite") }}</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <Separator />

    <!-- Outbox (audit) -->
    <div class="space-y-3">
      <h4 class="text-xs font-medium">
        {{ $t("bots.email.outbox") }}
      </h4>
      <div v-if="outboxLoading" class="flex items-center gap-2 text-xs text-muted-foreground p-4">
        <Spinner />
        <span>{{ $t("common.loading") }}</span>
      </div>
      <div v-else-if="!outboxItems?.length" class="rounded-md border p-4">
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.email.noEmails") }}
        </p>
      </div>
      <div v-else class="overflow-x-auto rounded-md border">
        <table class="w-full text-xs">
          <thead class="bg-muted/50 text-left">
            <tr>
              <th class="px-3 py-2 font-medium">
                {{ $t("bots.email.to") }}
              </th>
              <th class="px-3 py-2 font-medium">
                {{ $t("bots.email.subject") }}
              </th>
              <th class="px-3 py-2 font-medium">
                {{ $t("bots.email.status") }}
              </th>
              <th class="px-3 py-2 font-medium">
                {{ $t("bots.email.sentAt") }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in outboxItems" :key="item.id" class="border-t">
              <td class="px-3 py-2 text-xs">
                {{ Array.isArray(item.to) ? item.to.join(", ") : item.to }}
              </td>
              <td class="px-3 py-2">
                {{ item.subject }}
              </td>
              <td class="px-3 py-2">
                <Badge :variant="item.status === 'failed' ? 'destructive' : 'secondary'">
                  {{ item.status }}
                </Badge>
              </td>
              <td class="px-3 py-2 text-xs text-muted-foreground whitespace-nowrap">
                {{ formatDate(item.sent_at) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Badge,
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Separator,
  Spinner,
  Switch,
} from "@stringke/ui";
import ConfirmPopover from "@/components/confirm-popover/index.vue";
import { Plus } from "lucide-vue-next";
import { computed, ref } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useQuery, useQueryCache } from "@pinia/colada";
import type {
  EmailBinding,
  EmailOutboxItem,
  EmailProvider,
  TimestampMessage,
} from "@stringke/sdk/connect";
import { formatDateTime } from "@/utils/date-time";
import { connectClients } from "@/lib/connect-client";

const props = defineProps<{ botId: string }>();
const { t } = useI18n();

type EmailBindingView = EmailBinding & {
  can_read?: boolean;
  can_write?: boolean;
  can_delete?: boolean;
};

type EmailOutboxItemView = Omit<EmailOutboxItem, "sentAt"> & {
  sent_at?: string;
};

const queryCache = useQueryCache();

const { data: providersData } = useQuery({
  key: () => ["email-providers"],
  query: async () => {
    const response = await connectClients.emailProviders.listEmailProviders({});
    return response.providers;
  },
});

const {
  data: bindingsData,
  isLoading: bindingsLoading,
  refetch: refetchBindings,
} = useQuery({
  key: () => ["bot-email-bindings", props.botId],
  query: async () => {
    if (!props.botId) return [] as EmailBindingView[];
    const response = await connectClients.emailBindings.listEmailBindings({
      botId: props.botId,
    });
    return response.bindings.map(emailBindingFromProto);
  },
  enabled: () => !!props.botId,
});

const { data: outboxData, isLoading: outboxLoading } = useQuery({
  key: () => ["bot-email-outbox", props.botId],
  query: async () => {
    if (!props.botId) return [] as EmailOutboxItemView[];
    const response = await connectClients.emailOutbox.listEmailOutbox({
      botId: props.botId,
      page: { pageSize: 50 },
    });
    return response.items.map(emailOutboxFromProto);
  },
  enabled: () => !!props.botId,
});

const providers = computed<EmailProvider[]>(() => providersData.value ?? []);
const bindings = computed<EmailBindingView[]>(() => bindingsData.value ?? []);
const outboxItems = computed<EmailOutboxItemView[]>(() => outboxData.value ?? []);

const addingBinding = ref(false);
const addingProviderId = ref("");
const deletingId = ref("");

const providerNameMap = computed(() => {
  const map: Record<string, string> = {};
  for (const p of providers.value) {
    if (p.id && p.name) map[p.id] = p.name;
  }
  return map;
});

const unboundProviders = computed(() => {
  const boundIds = new Set(bindings.value.map((b) => b.providerId));
  return providers.value.filter((p) => !boundIds.has(p.id));
});

function invalidateBindings() {
  queryCache.invalidateQueries({ key: ["bot-email-bindings", props.botId] });
}

async function handleAddBinding(provider: EmailProvider) {
  addingBinding.value = true;
  addingProviderId.value = provider.id;
  const emailAddr =
    ((provider.config as Record<string, unknown>)?.username as string) || provider.name || "";
  try {
    await connectClients.emailBindings.createEmailBinding({
      botId: props.botId,
      providerId: provider.id,
      address: emailAddr,
      enabled: true,
      config: {
        can_read: true,
        can_write: true,
        can_delete: false,
      },
    });
    invalidateBindings();
    await refetchBindings();
    toast.success(t("bots.email.bindSuccess"));
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t("common.saveFailed"));
  } finally {
    addingBinding.value = false;
    addingProviderId.value = "";
  }
}

async function handleTogglePerm(binding: EmailBindingView, field: string, value: boolean) {
  try {
    await connectClients.emailBindings.updateEmailBinding({
      id: binding.id,
      address: binding.address,
      enabled: field === "can_read" ? value || !!binding.can_write : !!binding.can_read || value,
      config: {
        ...(binding.config as Record<string, unknown> | undefined),
        can_read: field === "can_read" ? value : !!binding.can_read,
        can_write: field === "can_write" ? value : !!binding.can_write,
        can_delete: !!binding.can_delete,
      },
    });
    invalidateBindings();
    await refetchBindings();
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t("common.saveFailed"));
  }
}

async function handleDeleteBinding(id: string) {
  deletingId.value = id;
  try {
    await connectClients.emailBindings.deleteEmailBinding({ id });
    invalidateBindings();
    await refetchBindings();
    toast.success(t("bots.email.unbindSuccess"));
  } catch (e: unknown) {
    toast.error(e instanceof Error ? e.message : t("common.saveFailed"));
  } finally {
    deletingId.value = "";
  }
}

function formatDate(value: string | undefined) {
  return formatDateTime(value, { fallback: "-" });
}

function emailBindingFromProto(binding: EmailBinding): EmailBindingView {
  const config = (binding.config ?? {}) as Record<string, unknown>;
  return {
    ...binding,
    can_read: config.can_read === true,
    can_write: config.can_write === true,
    can_delete: config.can_delete === true,
  };
}

function emailOutboxFromProto(item: EmailOutboxItem): EmailOutboxItemView {
  return {
    ...item,
    sent_at: timestampToISOString(item.sentAt),
  };
}

function timestampToISOString(value: TimestampMessage | undefined): string | undefined {
  if (!value) return undefined;
  const seconds = Number(value.seconds ?? 0n);
  const nanos = Number(value.nanos ?? 0);
  return new Date(seconds * 1000 + Math.floor(nanos / 1_000_000)).toISOString();
}
</script>
