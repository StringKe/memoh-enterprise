import type { RouteLocationNormalized } from "vue-router";

export function resolveAuthRedirect(to: Pick<RouteLocationNormalized, "fullPath" | "path">) {
  const token = localStorage.getItem("token");

  if (to.fullPath === "/login") {
    return token ? { path: "/" } : true;
  }
  if (to.path.startsWith("/oauth/")) {
    return true;
  }
  return token ? true : { name: "Login" };
}
