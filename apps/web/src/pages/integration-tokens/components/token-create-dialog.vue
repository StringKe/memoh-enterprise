<template>
  <FormDialogShell
    v-model:open="open"
    :title="$t('integrationTokens.createTitle')"
    :description="$t('integrationTokens.createDescription')"
    :cancel-text="$t('common.cancel')"
    :submit-text="$t('common.create')"
    :submit-disabled="!canCreate"
    :loading="creating"
    max-width-class="sm:max-w-2xl"
    @submit="handleSubmit"
  >
    <template #trigger>
      <Button>
        <KeyRound class="mr-1.5 size-3.5" />
        {{ $t("common.create") }}
      </Button>
    </template>

    <template #body>
      <div class="mt-4 grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ $t("common.name") }}</Label>
          <Input v-model="name" :placeholder="$t('integrationTokens.namePlaceholder')" />
        </div>

        <div class="space-y-2">
          <Label>{{ $t("integrationTokens.scope") }}</Label>
          <select
            v-model="scopeType"
            class="h-9 w-full rounded-md border bg-background px-3 text-sm"
          >
            <option value="global">{{ $t("integrationTokens.scopeGlobal") }}</option>
            <option value="bot">{{ $t("integrationTokens.scopeBot") }}</option>
            <option value="bot_group">{{ $t("integrationTokens.scopeBotGroup") }}</option>
          </select>
        </div>

        <div v-if="scopeType === 'bot'" class="space-y-2">
          <Label>{{ $t("integrationTokens.scopeBot") }}</Label>
          <Input v-model="scopeBotId" :placeholder="$t('integrationTokens.botIdPlaceholder')" />
        </div>

        <div v-if="scopeType === 'bot_group'" class="space-y-2">
          <Label>{{ $t("integrationTokens.scopeBotGroup") }}</Label>
          <Input
            v-model="scopeBotGroupId"
            :placeholder="$t('integrationTokens.botGroupIdPlaceholder')"
          />
        </div>

        <div class="space-y-2">
          <Label>{{ $t("integrationTokens.allowedEvents") }}</Label>
          <Input
            v-model="allowedEventTypes"
            :placeholder="$t('integrationTokens.allowedEventsPlaceholder')"
          />
        </div>

        <div class="space-y-2">
          <Label>{{ $t("integrationTokens.allowedActions") }}</Label>
          <Input
            v-model="allowedActionTypes"
            :placeholder="$t('integrationTokens.allowedActionsPlaceholder')"
          />
        </div>

        <div class="space-y-2">
          <Label>{{ $t("integrationTokens.expiresAt") }}</Label>
          <Input v-model="expiresAt" type="datetime-local" />
        </div>
      </div>
    </template>
  </FormDialogShell>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { Button, Input, Label } from "@stringke/ui";
import { KeyRound } from "lucide-vue-next";
import FormDialogShell from "@/components/form-dialog-shell/index.vue";
import { useConnectMutation } from "@/lib/connect-colada";
import { connectClients } from "@/lib/connect-client";
import { resolveApiErrorMessage } from "@/utils/api-error";
import {
  buildCreateIntegrationApiTokenRequest,
  canCreateIntegrationApiToken,
  type IntegrationTokenFormState,
} from "../token-form";

const emit = defineEmits<{
  created: [rawToken: string];
}>();

const { t: $t } = useI18n();
const open = ref(false);
const name = ref("");
const scopeType = ref("global");
const scopeBotId = ref("");
const scopeBotGroupId = ref("");
const allowedEventTypes = ref("");
const allowedActionTypes = ref("");
const expiresAt = ref("");

const formState = computed<IntegrationTokenFormState>(() => ({
  name: name.value,
  scopeType: scopeType.value,
  scopeBotId: scopeBotId.value,
  scopeBotGroupId: scopeBotGroupId.value,
  allowedEventTypes: allowedEventTypes.value,
  allowedActionTypes: allowedActionTypes.value,
  expiresAt: expiresAt.value,
}));

const canCreate = computed(() => canCreateIntegrationApiToken(formState.value));

const { mutateAsync: createToken, isLoading: creating } = useConnectMutation({
  mutation: () =>
    connectClients.integrationAdmin.createIntegrationApiToken(
      buildCreateIntegrationApiTokenRequest(formState.value),
    ),
});

async function handleSubmit() {
  if (!canCreate.value) return;
  try {
    const response = await createToken();
    emit("created", response.rawToken);
    resetForm();
    open.value = false;
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  }
}

function resetForm() {
  name.value = "";
  scopeType.value = "global";
  scopeBotId.value = "";
  scopeBotGroupId.value = "";
  allowedEventTypes.value = "";
  allowedActionTypes.value = "";
  expiresAt.value = "";
}
</script>
