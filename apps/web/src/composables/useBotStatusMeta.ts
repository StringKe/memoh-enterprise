import { computed, type Ref } from "vue";

interface BotStatusSource {
  status?: string | null;
  is_active?: boolean | null;
  isActive?: boolean | null;
  check_state?: string | null;
  checkState?: string | null;
  check_issue_count?: number | null;
  checkIssueCount?: number | null;
}

export function useBotStatusMeta(
  bot: Ref<BotStatusSource | null | undefined>,
  t: (key: string, named?: Record<string, unknown>) => string,
) {
  const isCreating = computed(() => bot.value?.status === "creating");
  const isDeleting = computed(() => bot.value?.status === "deleting");
  const isPending = computed(() => isCreating.value || isDeleting.value);
  const hasIssue = computed(() => (bot.value?.check_state ?? bot.value?.checkState) === "issue");

  const issueTitle = computed(() => {
    const count = Number(bot.value?.check_issue_count ?? bot.value?.checkIssueCount ?? 0);
    if (count <= 0) {
      return t("bots.checks.hasIssue");
    }
    return t("bots.checks.issueCount", { count });
  });

  const statusVariant = computed<"default" | "secondary" | "destructive">(() => {
    if (isPending.value) return "secondary";
    if (hasIssue.value) return "destructive";
    return (bot.value?.is_active ?? bot.value?.isActive) ? "default" : "secondary";
  });

  const statusLabel = computed(() => {
    if (isCreating.value) return t("bots.lifecycle.creating");
    if (isDeleting.value) return t("bots.lifecycle.deleting");
    if (hasIssue.value) return issueTitle.value;
    return (bot.value?.is_active ?? bot.value?.isActive) ? t("bots.active") : t("bots.inactive");
  });

  return {
    hasIssue,
    isPending,
    issueTitle,
    statusLabel,
    statusVariant,
  };
}
