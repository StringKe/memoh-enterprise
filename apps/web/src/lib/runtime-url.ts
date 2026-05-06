export function apiBaseUrl(): string {
  return import.meta.env.VITE_API_URL?.trim() || "/api";
}

export function apiHttpUrl(path: string): string {
  const baseUrl = apiBaseUrl();
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  if (!baseUrl || baseUrl.startsWith("/")) {
    return `${baseUrl || "/api"}${normalizedPath}`;
  }
  return new URL(normalizedPath, baseUrl).toString();
}

export function apiWebSocketUrl(path: string): string {
  const baseUrl = apiBaseUrl();
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  if (!baseUrl || baseUrl.startsWith("/")) {
    const loc = window.location;
    const proto = loc.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${loc.host}${(baseUrl || "/api").replace(/\/+$/, "")}${normalizedPath}`;
  }

  const url = new URL(normalizedPath, baseUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}
