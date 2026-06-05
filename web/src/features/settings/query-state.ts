"use client";

import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryKey
} from "@tanstack/react-query";
import { useCallback, useState } from "react";

import type { ApiResult } from "@/lib/api/client";

export type SettingsNotice = {
  kind: "info" | "warning" | "error";
  message: string;
};

type SettingsListOptions<TResponse, TItem> = {
  initialResult: ApiResult<TResponse>;
  queryKey: QueryKey;
  queryFn: () => Promise<ApiResult<TResponse>>;
  refreshMessage: string;
  selectItems: (response: TResponse) => TItem[];
};

type SettingsMutationOptions<TVariables, TData> = {
  invalidateQueryKey: QueryKey;
  mutationFn: (variables: TVariables) => Promise<ApiResult<TData>>;
};

class SettingsApiResultError extends Error {
  readonly status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = "SettingsApiResultError";
    this.status = status;
  }
}

export function settingsErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message;
  }
  return "Request failed.";
}

export function useSettingsList<TResponse, TItem>({
  initialResult,
  queryKey,
  queryFn,
  refreshMessage,
  selectItems
}: SettingsListOptions<TResponse, TItem>) {
  const [notice, setNotice] = useState<SettingsNotice | null>(
    initialResult.ok ? null : { kind: "error", message: initialResult.error.message }
  );
  const query = useQuery<TResponse, SettingsApiResultError>({
    initialData: initialResult.ok ? initialResult.data : undefined,
    queryKey,
    queryFn: () => unwrapApiResult(queryFn())
  });
  const { refetch } = query;

  const refresh = useCallback(async () => {
    const refreshed = await refetch();
    if (refreshed.error) {
      setNotice({ kind: "error", message: settingsErrorMessage(refreshed.error) });
      return;
    }
    setNotice({ kind: "info", message: refreshMessage });
  }, [refetch, refreshMessage]);

  return {
    items: query.data === undefined ? [] : selectItems(query.data),
    notice,
    query,
    refresh,
    setNotice
  };
}

export function useSettingsMutation<TVariables, TData>({
  invalidateQueryKey,
  mutationFn
}: SettingsMutationOptions<TVariables, TData>) {
  const queryClient = useQueryClient();
  return useMutation<TData, SettingsApiResultError, TVariables>({
    mutationFn: (variables) => unwrapApiResult(mutationFn(variables)),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invalidateQueryKey })
  });
}

async function unwrapApiResult<T>(request: Promise<ApiResult<T>>): Promise<T> {
  const result = await request;
  if (!result.ok) {
    throw new SettingsApiResultError(result.error.message, result.error.status);
  }
  return result.data;
}
