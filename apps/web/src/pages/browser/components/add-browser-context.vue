<template>
  <section>
    <FormDialogShell
      v-model:open="open"
      :title="$t('browser.add')"
      :cancel-text="$t('common.cancel')"
      :submit-text="$t('browser.add')"
      :submit-disabled="form.meta.value.valid === false || isLoading"
      :loading="isLoading"
      @submit="handleCreate"
    >
      <template #trigger>
        <Button class="w-full shadow-none! text-muted-foreground mb-4" variant="outline">
          <Plus class="mr-1" /> {{ $t("browser.add") }}
        </Button>
      </template>
      <template #body>
        <div class="flex-col gap-3 flex mt-4">
          <FormField v-slot="{ componentField }" name="name">
            <FormItem>
              <Label :for="'browser-context-name'">
                {{ $t("browser.name") }}
              </Label>
              <FormControl>
                <Input
                  :id="'browser-context-name'"
                  type="text"
                  :placeholder="$t('browser.namePlaceholder')"
                  v-bind="componentField"
                />
              </FormControl>
            </FormItem>
          </FormField>
        </div>
      </template>
    </FormDialogShell>
  </section>
</template>

<script setup lang="ts">
import { Button, Input, FormField, FormControl, FormItem, Label } from "@stringke/ui";
import { toTypedSchema } from "@vee-validate/zod";
import z from "zod";
import { useForm } from "vee-validate";
import { useMutation, useQueryCache } from "@pinia/colada";
import { useI18n } from "vue-i18n";
import { Plus } from "lucide-vue-next";
import FormDialogShell from "@/components/form-dialog-shell/index.vue";
import { connectClients } from "@/lib/connect-client";
import { resolveConnectErrorMessage } from "@/lib/connect-errors";
import { toast } from "vue-sonner";

const open = defineModel<boolean>("open");
const { t } = useI18n();

const queryCache = useQueryCache();
const { mutateAsync: createMutation, isLoading } = useMutation({
  mutation: (data: { name: string }) =>
    connectClients.browserContexts.createBrowserContext({ name: data.name }),
  onSettled: () =>
    queryCache.invalidateQueries({ key: ["browser-contexts"] }).catch((err) => {
      console.error(err);
    }),
  onError: (_, __, previous) => {
    queryCache.setQueryData(["browser-contexts"], previous);
  },
});

const schema = toTypedSchema(
  z.object({
    name: z.string().min(1),
  }),
);

const form = useForm({ validationSchema: schema });

const handleCreate = form.handleSubmit(async (value) => {
  try {
    await createMutation(value);
    open.value = false;
  } catch (err) {
    toast.error(resolveConnectErrorMessage(err, t("common.saveFailed")));
  }
});
</script>
