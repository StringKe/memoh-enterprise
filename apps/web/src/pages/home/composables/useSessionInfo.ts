import { computed, ref, type Ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuery } from "@pinia/colada";
import { useChatStore } from "@/store/chat-list";
import { connectClients } from "@/lib/connect-client";
import { recordValue } from "@/lib/connect-runtime";

export interface SessionInfoResponse {
  context_usage?: {
    used_tokens?: number;
    context_window?: number;
  };
  cache_stats?: {
    cache_read_tokens?: number;
    total_input_tokens?: number;
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
  void modelId;
  const response = await connectClients.bots.readBotSessionHistory({
    botId,
    sessionId,
    limit: 200,
    beforeMessageId: "",
  });

  let usedTokens = 0;
  let cacheReadTokens = 0;
  let totalInputTokens = 0;
  const skills = new Set<string>();

  for (const message of response.messages) {
    const payload = recordValue(message.payload);
    const usage = recordValue(payload.usage);
    const metadata = recordValue(payload.metadata);
    const tokenValue = usage.total_tokens ?? usage.totalTokens ?? metadata.total_tokens;
    if (typeof tokenValue === "number") {
      usedTokens = tokenValue;
    }
    const cacheRead =
      usage.cache_read_tokens ?? usage.cacheReadTokens ?? metadata.cache_read_tokens;
    if (typeof cacheRead === "number") {
      cacheReadTokens = cacheRead;
    }
    const totalInput =
      usage.total_input_tokens ?? usage.totalInputTokens ?? metadata.total_input_tokens;
    if (typeof totalInput === "number") {
      totalInputTokens = totalInput;
    }
    const usedSkills = payload.skills ?? metadata.skills;
    if (Array.isArray(usedSkills)) {
      for (const item of usedSkills) {
        if (typeof item === "string" && item.trim()) {
          skills.add(item.trim());
        }
      }
    }
  }

  return {
    message_count: response.messages.length,
    context_usage: {
      used_tokens: usedTokens,
    },
    cache_stats: {
      cache_read_tokens: cacheReadTokens,
      total_input_tokens: totalInputTokens,
      cache_hit_rate: totalInputTokens > 0 ? (cacheReadTokens / totalInputTokens) * 100 : 0,
    },
    skills: [...skills],
  };
}
