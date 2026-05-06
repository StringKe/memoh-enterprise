import type { Interceptor, Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { createMemohConnectClients } from "@stringke/sdk/connect";

export function createAuthInterceptor(getToken = () => localStorage.getItem("token")): Interceptor {
  return (next) => async (req) => {
    const token = getToken();
    if (token) {
      req.header.set("Authorization", `Bearer ${token}`);
    }
    return await next(req);
  };
}

export function createMemohWebTransport(baseUrl: string): Transport {
  return createConnectTransport({
    baseUrl,
    interceptors: [createAuthInterceptor()],
    fetch: (input, init) => fetch(input, { ...init, credentials: "include" }),
  });
}

const connectBaseUrl = import.meta.env.VITE_CONNECT_URL?.trim() || "/connect";

const transport = createMemohWebTransport(connectBaseUrl);

export const connectClients = createMemohConnectClients({
  baseUrl: connectBaseUrl,
  transport,
});
