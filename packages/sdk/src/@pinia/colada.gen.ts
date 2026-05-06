import { defineQueryOptions, type _JSONValue, type UseMutationOptions } from "@pinia/colada";
import type { CallOptions } from "@connectrpc/connect";

export type ConnectQueryKeyPart = _JSONValue;
export type ConnectQueryKey = readonly ConnectQueryKeyPart[];
export type ConnectUnaryFn<TRequest, TResponse> = (
  request: TRequest,
  options?: CallOptions,
) => Promise<TResponse>;

export type ConnectQueryOptionsInput<TData> = {
  key: ConnectQueryKey;
  query: (options?: CallOptions) => Promise<TData>;
  callOptions?: CallOptions;
};

export type ConnectQueryOptionsWithParamsInput<TParams, TData> = {
  key: (params: TParams) => ConnectQueryKey;
  query: (params: TParams, options?: CallOptions) => Promise<TData>;
  callOptions?: (params: TParams) => CallOptions | undefined;
};

export function connectQueryKey(...parts: ConnectQueryKeyPart[]): ConnectQueryKey {
  return parts;
}

export function connectQueryOptions<TData>({
  key,
  query,
  callOptions,
}: ConnectQueryOptionsInput<TData>) {
  return defineQueryOptions({
    key: [...key],
    query: () => query(callOptions),
  });
}

export function connectQueryOptionsWithParams<TParams, TData>({
  key,
  query,
  callOptions,
}: ConnectQueryOptionsWithParamsInput<TParams, TData>) {
  return defineQueryOptions((params: TParams) => ({
    key: [...key(params)],
    query: () => query(params, callOptions?.(params)),
  }));
}

export function connectUnaryQueryOptions<TRequest, TResponse>(
  key: ConnectQueryKey,
  rpc: ConnectUnaryFn<TRequest, TResponse>,
  request: TRequest,
  callOptions?: CallOptions,
) {
  return connectQueryOptions({
    key,
    query: () => rpc(request, callOptions),
  });
}

export function connectMutationOptions<
  TData,
  TVars = void,
  TError = Error,
  TContext extends Record<PropertyKey, unknown> = Record<PropertyKey, never>,
>(
  mutation: (vars: TVars, options?: CallOptions) => Promise<TData>,
  options: Omit<UseMutationOptions<TData, TVars, TError, TContext>, "mutation"> & {
    callOptions?: CallOptions;
  } = {},
): UseMutationOptions<TData, TVars, TError, TContext> {
  const { callOptions, ...mutationOptions } = options;

  return {
    ...mutationOptions,
    mutation: (vars) => mutation(vars, callOptions),
  };
}
