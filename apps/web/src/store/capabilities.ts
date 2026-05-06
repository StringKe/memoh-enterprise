import { defineStore } from "pinia";
import { ref } from "vue";

type PingResponse = {
  container_backend?: string;
  local_workspace_enabled?: boolean;
  snapshot_supported?: boolean;
  version?: string;
  commit_hash?: string;
};

export const useCapabilitiesStore = defineStore("capabilities", () => {
  const containerBackend = ref("containerd");
  const localWorkspaceEnabled = ref(false);
  const snapshotSupported = ref(true);
  const serverVersion = ref("");
  const commitHash = ref("");
  const loaded = ref(false);

  async function load() {
    if (loaded.value) return;
    try {
      const response = await fetch("/ping");
      if (!response.ok) return;
      const data = (await response.json()) as PingResponse;
      containerBackend.value = data.container_backend ?? "containerd";
      localWorkspaceEnabled.value = data.local_workspace_enabled === true;
      snapshotSupported.value = data.snapshot_supported !== false;
      serverVersion.value = data.version ?? "";
      commitHash.value = data.commit_hash ?? "";
    } catch {
      // fallback: assume containerd
    }
    loaded.value = true;
  }

  return {
    containerBackend,
    localWorkspaceEnabled,
    snapshotSupported,
    serverVersion,
    commitHash,
    loaded,
    load,
  };
});
