<template>
  <section class="p-4 mx-auto space-y-4">
    <div class="flex items-center justify-between gap-3 flex-wrap">
      <h2 class="text-xs font-medium">{{ $t("integrationTokens.title") }}</h2>
      <div class="flex items-center gap-2">
        <TokenCreateDialog @created="handleCreated" />
      </div>
    </div>

    <div v-if="rawTokenDisplay.visible" class="rounded-md border bg-muted/30 p-3 text-sm">
      <div class="mb-2 font-medium">{{ $t("integrationTokens.rawToken") }}</div>
      <div class="flex items-center gap-2">
        <code class="flex-1 overflow-x-auto rounded bg-background px-2 py-1">
          {{ rawTokenDisplay.value }}
        </code>
        <Button variant="outline" size="sm" @click="copyRawToken">
          <Copy class="mr-1.5 size-3.5" />
          {{ $t("common.copy") }}
        </Button>
      </div>
    </div>

    <div v-if="loading" class="text-sm text-muted-foreground">
      {{ $t("common.loading") }}
    </div>
    <div v-else-if="tokens.length === 0" class="py-16 text-center text-sm text-muted-foreground">
      {{ $t("integrationTokens.empty") }}
    </div>
    <div v-else class="overflow-hidden border rounded-md">
      <table class="w-full text-sm">
        <thead class="bg-muted/50 text-left">
          <tr>
            <th class="px-3 py-2 font-medium">{{ $t("common.name") }}</th>
            <th class="px-3 py-2 font-medium">{{ $t("integrationTokens.scope") }}</th>
            <th class="px-3 py-2 font-medium">{{ $t("common.status") }}</th>
            <th class="px-3 py-2 font-medium text-right">{{ $t("common.operation") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="token in tokens" :key="token.id" class="border-t">
            <td class="px-3 py-2 font-medium">{{ token.name }}</td>
            <td class="px-3 py-2 text-muted-foreground">
              {{ token.scopeType }}
              <span v-if="token.scopeBotId">: {{ token.scopeBotId }}</span>
              <span v-if="token.scopeBotGroupId">: {{ token.scopeBotGroupId }}</span>
            </td>
            <td class="px-3 py-2">
              {{
                token.disabledAt
                  ? $t("integrationTokens.disabled")
                  : $t("integrationTokens.enabled")
              }}
            </td>
            <td class="px-3 py-2 text-right">
              <Button
                variant="ghost"
                size="sm"
                :disabled="Boolean(token.disabledAt)"
                @click="disableToken(token.id)"
              >
                <Ban class="size-3.5" />
              </Button>
              <Button variant="ghost" size="sm" @click="deleteToken(token.id)">
                <Trash2 class="size-3.5" />
              </Button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { Button } from "@stringke/ui";
import { Ban, Copy, Trash2 } from "lucide-vue-next";
import { useConnectMutation, useConnectQuery } from "@/lib/connect-colada";
import { connectClients } from "@/lib/connect-client";
import TokenCreateDialog from "./components/token-create-dialog.vue";
import { getRawTokenDisplay } from "./token-form";

const rawToken = ref("");
const rawTokenDisplay = computed(() => getRawTokenDisplay(rawToken.value));

const {
  data: tokensData,
  isLoading: loading,
  refetch: loadTokens,
} = useConnectQuery({
  key: ["integration-api-tokens"],
  query: () => connectClients.integrationAdmin.listIntegrationApiTokens({}),
});

const tokens = computed(() => tokensData.value?.tokens ?? []);

const { mutateAsync: disableTokenMutation } = useConnectMutation({
  mutation: (id: string) => connectClients.integrationAdmin.disableIntegrationApiToken({ id }),
});

const { mutateAsync: deleteTokenMutation } = useConnectMutation({
  mutation: (id: string) => connectClients.integrationAdmin.deleteIntegrationApiToken({ id }),
});

async function handleCreated(token: string) {
  rawToken.value = token;
  await loadTokens();
}

async function disableToken(id: string) {
  await disableTokenMutation(id);
  await loadTokens();
}

async function deleteToken(id: string) {
  await deleteTokenMutation(id);
  await loadTokens();
}

async function copyRawToken() {
  await navigator.clipboard.writeText(rawToken.value);
}
</script>
