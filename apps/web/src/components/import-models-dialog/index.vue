<template>
  <FormDialogShell
    v-model:open="open"
    :title="$t('models.importModels')"
    :cancel-text="$t('common.cancel')"
    :submit-text="$t('common.import')"
    :submit-disabled="false"
    :loading="isLoading"
    @submit="handleImport"
  >
    <template #trigger>
      <Button variant="outline" class="flex items-center gap-2">
        <FileInput />
        {{ $t("models.importModels") }}
      </Button>
    </template>
    <template #body>
      <div class="flex flex-col gap-3 mt-4">
        <p class="text-xs text-muted-foreground">
          {{ $t("models.importConfirmHint") }}
        </p>
      </div>
    </template>
  </FormDialogShell>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useI18n } from "vue-i18n";
import { useMutation, useQueryCache } from "@pinia/colada";
import { toast } from "vue-sonner";
import { FileInput } from "lucide-vue-next";
import { Button } from "@stringke/ui";
import FormDialogShell from "@/components/form-dialog-shell/index.vue";
import { connectClients } from "@/lib/connect-client";
import { resolveConnectErrorMessage } from "@/lib/connect-errors";

const props = defineProps<{
  providerId: string;
}>();

const open = ref(false);
const { t } = useI18n();
const queryCache = useQueryCache();

const { mutateAsync: importModelsMutation, isLoading } = useMutation({
  mutation: async () => {
    return await connectClients.providers.importProviderModels({ id: props.providerId });
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ["provider-models"] });
    queryCache.invalidateQueries({ key: ["models"] });
  },
});

async function handleImport() {
  try {
    const data = await importModelsMutation();
    if (data) {
      toast.success(
        t("models.importSuccess", {
          created: data.models.length,
          skipped: 0,
        }),
      );
    }
    open.value = false;
  } catch (error) {
    toast.error(resolveConnectErrorMessage(error, t("models.importFailed")));
  }
}
</script>
