import { computed, ref, type Ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuery } from "@pinia/colada";
import { useChatStore } from "@/store/chat-list";
import { apiHttpUrl } from "@/lib/runtime-url";

export interface SessionInfoResponse {
  context_usage?: {
    used_tokens?: number;
    context_window?: number;
  };
  cache_stats?: {
    cache_hit_rate?: number;
  };
  skills?: string[];
  [key: string]: unknown;
}

interface UseSessionInfoOptions {
  visible?: Ref<boolean>;
  overrideModelId?: Ref<string>;
}

export function useSessionInfo(options: UseSessionInfoOptions = {}) {
  const chatStore = useChatStore();
  const { currentBotId, sessionId } = storeToRefs(chatStore);
  const visible = options.visible ?? ref(true);

  const { data: info } = useQuery({
    key: () => [
      "session-status",
      currentBotId.value ?? "",
      sessionId.value ?? "",
      options.overrideModelId?.value ?? "",
    ],
    query: async () => {
      return fetchSessionInfo(
        currentBotId.value!,
        sessionId.value!,
        options.overrideModelId?.value || undefined,
      );
    },
    enabled: () => !!currentBotId.value && !!sessionId.value && visible.value,
    refetchOnWindowFocus: false,
  });

  const usedTokens = computed(() => info.value?.context_usage?.used_tokens ?? 0);
  const contextWindow = computed(() => info.value?.context_usage?.context_window ?? null);
  const contextPercent = computed(() => {
    if (contextWindow.value == null || contextWindow.value <= 0) return 0;
    return (usedTokens.value / contextWindow.value) * 100;
  });

  return {
    info,
    usedTokens,
    contextWindow,
    contextPercent,
    currentBotId,
    sessionId,
  };
}

async function fetchSessionInfo(
  botId: string,
  sessionId: string,
  modelId?: string,
): Promise<SessionInfoResponse> {
  const query = new URLSearchParams();
  if (modelId) query.set("model_id", modelId);
  const queryString = query.toString();
  const url = `${apiHttpUrl(
    `/bots/${encodeURIComponent(botId)}/sessions/${encodeURIComponent(sessionId)}/status`,
  )}${queryString ? `?${queryString}` : ""}`;
  const token = localStorage.getItem("token") || "";
  const response = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  });
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  return response.json() as Promise<SessionInfoResponse>;
}
