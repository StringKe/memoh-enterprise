import { defineStore } from "pinia";
import { reactive, watch } from "vue";
import { useLocalStorage } from "@vueuse/core";
import { useRouter } from "vue-router";
import { connectClients } from "@/lib/connect-client";

export interface UserInfo {
  id: string;
  username: string;
  role: string;
  displayName: string;
  avatarUrl: string;
  timezone: string;
}

export const useUserStore = defineStore(
  "user",
  () => {
    const userInfo = reactive<UserInfo>({
      id: "",
      username: "",
      role: "",
      displayName: "",
      avatarUrl: "",
      timezone: "UTC",
    });

    const localToken = useLocalStorage("token", "");

    const login = (userData: UserInfo, token: string) => {
      localToken.value = token;
      for (const key of Object.keys(userData) as (keyof UserInfo)[]) {
        userInfo[key] = userData[key];
      }
    };

    const patchUserInfo = (patch: Partial<UserInfo>) => {
      for (const key of Object.keys(patch) as (keyof UserInfo)[]) {
        const value = patch[key];
        if (value !== undefined) {
          userInfo[key] = value;
        }
      }
    };

    const syncCurrentUser = async () => {
      const response = await connectClients.auth.getMe({});
      if (!response.user) return;
      patchUserInfo({
        id: response.user.id,
        username: response.user.username,
        displayName: response.user.displayName,
        avatarUrl: response.user.avatarUrl,
        timezone: response.user.timezone || "UTC",
      });
    };

    const exitLogin = () => {
      localToken.value = "";
      for (const key of Object.keys(userInfo) as (keyof UserInfo)[]) {
        userInfo[key as keyof UserInfo] = key === "timezone" ? "UTC" : "";
      }
    };
    const router = useRouter();
    watch(
      localToken,
      () => {
        if (!localToken.value) {
          exitLogin();
          router.replace({ name: "Login" });
        }
      },
      {
        immediate: true,
      },
    );
    return {
      userInfo,
      login,
      patchUserInfo,
      syncCurrentUser,
      exitLogin,
    };
  },
  {
    persist: true,
  },
);
