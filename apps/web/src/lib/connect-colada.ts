import {
  type EntryKey,
  type UseMutationOptions,
  type UseMutationReturn,
  type UseQueryOptions,
  type UseQueryReturn,
  useMutation,
  useQuery,
} from "@pinia/colada";
import type { MaybeRefOrGetter } from "vue";

export type ConnectQueryOptions<
  TData,
  TError = Error,
  TDataInitial extends TData | undefined = undefined,
> = Omit<UseQueryOptions<TData, TError, TDataInitial>, "key" | "query"> & {
  key: MaybeRefOrGetter<EntryKey>;
  query: () => Promise<TData>;
};

export function useConnectQuery<
  TData,
  TError = Error,
  TDataInitial extends TData | undefined = undefined,
>(
  options: ConnectQueryOptions<TData, TError, TDataInitial>,
): UseQueryReturn<TData, TError, TDataInitial> {
  return useQuery(options);
}

export type ConnectMutationOptions<
  TData,
  TVars = void,
  TError = Error,
  TContext extends Record<string, unknown> = Record<string, never>,
> = Omit<UseMutationOptions<TData, TVars, TError, TContext>, "mutation"> & {
  mutation: (vars: TVars) => Promise<TData>;
};

export function useConnectMutation<
  TData,
  TVars = void,
  TError = Error,
  TContext extends Record<string, unknown> = Record<string, never>,
>(
  options: ConnectMutationOptions<TData, TVars, TError, TContext>,
): UseMutationReturn<TData, TVars, TError, TContext> {
  return useMutation(options);
}

export type ConnectServerStreamOptions<TEvent> = {
  signal?: AbortSignal;
  stream: (signal: AbortSignal) => AsyncIterable<TEvent>;
  onEvent: (event: TEvent) => void | Promise<void>;
};

export async function consumeConnectServerStream<TEvent>({
  signal,
  stream,
  onEvent,
}: ConnectServerStreamOptions<TEvent>): Promise<void> {
  const controller = new AbortController();
  const abort = () => controller.abort(signal?.reason);
  if (signal?.aborted) {
    abort();
  } else {
    signal?.addEventListener("abort", abort, { once: true });
  }

  try {
    for await (const event of stream(controller.signal)) {
      if (controller.signal.aborted) break;
      await onEvent(event);
    }
  } finally {
    signal?.removeEventListener("abort", abort);
  }
}
