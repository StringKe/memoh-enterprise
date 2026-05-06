import { ref } from "vue";
import type { SupermarketMcpEntry } from "@/pages/supermarket/supermarket-items";

const pendingDraft = ref<SupermarketMcpEntry | null>(null);

export function useSupermarketMcpDraft() {
  function setPendingDraft(entry: SupermarketMcpEntry) {
    pendingDraft.value = entry;
  }

  function consumePendingDraft(): SupermarketMcpEntry | null {
    const draft = pendingDraft.value;
    pendingDraft.value = null;
    return draft;
  }

  return { pendingDraft, setPendingDraft, consumePendingDraft };
}
