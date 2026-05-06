<template>
  <section class="mx-auto max-w-2xl p-4">
    <h2 class="mb-6 text-lg font-semibold">
      {{ $t("botGroups.createTitle") }}
    </h2>

    <form class="space-y-5" @submit.prevent="handleSubmit">
      <div class="space-y-2">
        <Label>{{ $t("common.name") }}</Label>
        <Input v-model="form.name" :placeholder="$t('botGroups.namePlaceholder')" />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("botGroups.description") }}</Label>
        <Textarea
          v-model="form.description"
          :placeholder="$t('botGroups.descriptionPlaceholder')"
        />
      </div>

      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" @click="router.back()">
          {{ $t("common.cancel") }}
        </Button>
        <Button type="submit" :disabled="!form.name.trim() || creating">
          <Spinner v-if="creating" class="mr-1.5" />
          {{ $t("common.create") }}
        </Button>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { reactive } from "vue";
import { useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useMutation, useQueryCache } from "@pinia/colada";
import { Button, Input, Label, Spinner, Textarea } from "@stringke/ui";
import { connectClients } from "@/lib/connect-client";
import { resolveApiErrorMessage } from "@/utils/api-error";

const router = useRouter();
const { t: $t } = useI18n();
const queryCache = useQueryCache();

const form = reactive({
  name: "",
  description: "",
});

const { mutateAsync: createGroup, isLoading: creating } = useMutation({
  mutation: async () =>
    connectClients.botGroups.createBotGroup({
      name: form.name.trim(),
      description: form.description.trim(),
    }),
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-groups"] }),
});

async function handleSubmit() {
  if (!form.name.trim()) return;
  try {
    const response = await createGroup();
    toast.success($t("botGroups.createSuccess"));
    const id = response.group?.id;
    if (id) {
      await router.push({ name: "bot-group-detail", params: { groupId: id } });
      return;
    }
    await router.push({ name: "bot-groups" });
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  }
}
</script>
