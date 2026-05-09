<template>
  <section class="max-w-7xl mx-auto p-4 pb-12">
    <div class="max-w-3xl mx-auto space-y-8">
      <!-- Avatar & name -->
      <div class="flex items-center gap-4">
        <Avatar class="size-14 shrink-0">
          <AvatarImage
            v-if="profileForm.avatar_url"
            :src="profileForm.avatar_url"
            :alt="displayTitle"
          />
          <AvatarFallback>
            {{ avatarFallback }}
          </AvatarFallback>
        </Avatar>
        <div class="min-w-0">
          <p class="text-xs font-medium truncate">
            {{ displayTitle }}
          </p>
          <p class="text-xs text-muted-foreground truncate">
            {{ displayUserID }}
          </p>
        </div>
      </div>

      <!-- Logout -->
      <section>
        <Separator class="mb-4" />
        <ConfirmPopover :message="$t('auth.logoutConfirm')" @confirm="onLogout">
          <template #trigger>
            <Button>
              {{ $t("auth.logout") }}
            </Button>
          </template>
        </ConfirmPopover>
      </section>

      <ProfileSection
        :display-user-id="displayUserID"
        :display-username="displayUsername"
        :display-name="profileForm.display_name"
        :avatar-url="profileForm.avatar_url"
        :timezone="profileForm.timezone"
        :saving="savingProfile"
        :loading="loadingInitial"
        @update:display-name="profileForm.display_name = $event"
        @update:avatar-url="profileForm.avatar_url = $event"
        @update:timezone="profileForm.timezone = $event"
        @save="onSaveProfile"
      />

      <PasswordSection
        :current-password="passwordForm.currentPassword"
        :new-password="passwordForm.newPassword"
        :confirm-password="passwordForm.confirmPassword"
        :saving="savingPassword"
        :loading="loadingInitial"
        @update:current-password="passwordForm.currentPassword = $event"
        @update:new-password="passwordForm.newPassword = $event"
        @update:confirm-password="passwordForm.confirmPassword = $event"
        @update-password="onUpdatePassword"
      />

      <!-- Linked Channels -->
      <section>
        <h2 class="mb-2 flex items-center text-xs font-medium">
          <Network class="mr-2 size-3.5" />
          {{ $t("settings.linkedChannels") }}
        </h2>
        <Separator />
        <div class="mt-4 space-y-3">
          <p v-if="loadingIdentities" class="text-xs text-muted-foreground">
            {{ $t("common.loading") }}
          </p>
          <p v-else-if="identities.length === 0" class="text-xs text-muted-foreground">
            {{ $t("settings.noLinkedChannels") }}
          </p>
          <template v-else>
            <div
              v-for="identity in identities"
              :key="identity.id"
              class="border rounded-md p-3 space-y-1"
            >
              <div class="flex items-center justify-between gap-3">
                <p class="text-xs font-medium truncate">
                  {{ identity.display_name || identity.channel_subject_id }}
                </p>
                <Badge variant="secondary">
                  {{ platformLabel(identity.channel) }}
                </Badge>
              </div>
              <p class="text-xs text-muted-foreground truncate">
                {{ identity.channel_subject_id }}
              </p>
              <p class="text-xs text-muted-foreground truncate">
                {{ identity.id }}
              </p>
            </div>
          </template>
        </div>
      </section>

    </div>
  </section>
</template>

<script setup lang="ts">
import { Avatar, AvatarFallback, AvatarImage, Badge, Button, Separator } from "@stringke/ui";
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { Network } from "lucide-vue-next";
import ConfirmPopover from "@/components/confirm-popover/index.vue";
import ProfileSection from "./components/profile-section.vue";
import PasswordSection from "./components/password-section.vue";
import type {
  User as ConnectUser,
  UserIdentity as ConnectUserIdentity,
} from "@stringke/sdk/connect";
import { connectClients } from "@/lib/connect-client";
import { resolveConnectErrorMessage } from "@/lib/connect-errors";
import { useUserStore } from "@/store/user";
import { useAvatarInitials } from "@/composables/useAvatarInitials";
import { channelTypeDisplayName } from "@/utils/channel-type-label";

type UserAccount = {
  id: string;
  username: string;
  email: string;
  role: string;
  display_name: string;
  avatar_url: string;
  timezone: string;
};

type LinkedIdentity = {
  id: string;
  channel: string;
  channel_subject_id: string;
  display_name: string;
};

const { t } = useI18n();
const router = useRouter();
const userStore = useUserStore();
const { userInfo, exitLogin, patchUserInfo } = userStore;

// ---- User data ----
const account = ref<UserAccount | null>(null);
const identities = ref<LinkedIdentity[]>([]);

const loadingInitial = ref(false);
const loadingIdentities = ref(false);
const savingProfile = ref(false);
const savingPassword = ref(false);

const profileForm = reactive({
  display_name: "",
  avatar_url: "",
  timezone: "",
});

const passwordForm = reactive({
  currentPassword: "",
  newPassword: "",
  confirmPassword: "",
});

const displayUserID = computed(() => account.value?.id || userInfo.id || "");
const displayUsername = computed(() => account.value?.username || userInfo.username || "");
const displayTitle = computed(() => {
  return (
    profileForm.display_name.trim() ||
    displayUsername.value ||
    displayUserID.value ||
    t("settings.user")
  );
});
const avatarFallback = useAvatarInitials(() => displayTitle.value, "U");

function platformLabel(platformKey: string): string {
  if (!platformKey?.trim()) return platformKey ?? "";
  return channelTypeDisplayName(t, platformKey, null) || platformKey;
}

onMounted(() => {
  void loadPageData();
});

async function loadPageData() {
  loadingInitial.value = true;
  try {
    await Promise.all([loadMyAccount(), loadMyIdentities()]);
  } catch {
    toast.error(t("settings.loadUserFailed"));
  } finally {
    loadingInitial.value = false;
  }
}

async function loadMyAccount() {
  const response = await connectClients.users.getCurrentUser({});
  const data = toUserAccount(response.user);
  account.value = data;
  profileForm.display_name = data.display_name;
  profileForm.avatar_url = data.avatar_url;
  profileForm.timezone = data.timezone;
  patchUserInfo({
    id: data.id,
    username: data.username,
    role: data.role,
    displayName: data.display_name,
    avatarUrl: data.avatar_url,
    timezone: data.timezone,
  });
}

async function loadMyIdentities() {
  loadingIdentities.value = true;
  try {
    const response = await connectClients.users.listMyIdentities({});
    identities.value = response.identities.map(toLinkedIdentity);
  } finally {
    loadingIdentities.value = false;
  }
}

async function onSaveProfile() {
  savingProfile.value = true;
  try {
    const response = await connectClients.users.updateCurrentUser({
      displayName: profileForm.display_name.trim(),
      avatarUrl: profileForm.avatar_url.trim(),
      timezone: profileForm.timezone.trim(),
    });
    const data = toUserAccount(response.user);
    account.value = data;
    profileForm.display_name = data.display_name;
    profileForm.avatar_url = data.avatar_url;
    profileForm.timezone = data.timezone;
    patchUserInfo({
      displayName: data.display_name,
      avatarUrl: data.avatar_url,
      timezone: data.timezone,
    });
    toast.success(t("settings.profileUpdated"));
  } catch (error) {
    toast.error(resolveConnectErrorMessage(error, t("settings.profileUpdateFailed")));
  } finally {
    savingProfile.value = false;
  }
}

async function onUpdatePassword() {
  const currentPassword = passwordForm.currentPassword.trim();
  const newPassword = passwordForm.newPassword.trim();
  const confirmPassword = passwordForm.confirmPassword.trim();
  if (!currentPassword || !newPassword) {
    toast.error(t("settings.passwordRequired"));
    return;
  }
  if (newPassword !== confirmPassword) {
    toast.error(t("settings.passwordNotMatch"));
    return;
  }
  savingPassword.value = true;
  try {
    await connectClients.users.updateCurrentUserPassword({
      currentPassword,
      newPassword,
    });
    passwordForm.currentPassword = "";
    passwordForm.newPassword = "";
    passwordForm.confirmPassword = "";
    toast.success(t("settings.passwordUpdated"));
  } catch (error) {
    toast.error(resolveConnectErrorMessage(error, t("settings.passwordUpdateFailed")));
  } finally {
    savingPassword.value = false;
  }
}

function onLogout() {
  exitLogin();
  void router.replace({ name: "Login" });
}

function toUserAccount(user?: ConnectUser): UserAccount {
  return {
    id: user?.id ?? "",
    username: user?.username ?? "",
    email: user?.email ?? "",
    role: user?.role ?? "",
    display_name: user?.displayName ?? "",
    avatar_url: user?.avatarUrl ?? "",
    timezone: user?.timezone || "UTC",
  };
}

function toLinkedIdentity(identity: ConnectUserIdentity): LinkedIdentity {
  return {
    id: identity.id,
    channel: identity.channel,
    channel_subject_id: identity.externalId,
    display_name: identity.displayName,
  };
}
</script>
