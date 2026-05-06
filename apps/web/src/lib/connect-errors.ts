import { Code, ConnectError } from "@connectrpc/connect";

const codeMessages: Partial<Record<Code, string>> = {
  [Code.Unauthenticated]: "Unauthenticated",
  [Code.PermissionDenied]: "Permission denied",
  [Code.NotFound]: "Not found",
  [Code.InvalidArgument]: "Invalid request",
  [Code.Unavailable]: "Service unavailable",
};

export function resolveConnectErrorMessage(error: unknown, fallback = "Request failed"): string {
  if (error instanceof ConnectError) {
    return error.rawMessage || codeMessages[error.code] || fallback;
  }
  if (error instanceof Error && error.message.trim()) {
    return error.message;
  }
  return fallback;
}
