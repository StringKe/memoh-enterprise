export function connectBaseUrl(): string {
  return import.meta.env.VITE_CONNECT_URL?.trim() || "/connect";
}
