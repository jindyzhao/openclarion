"use client";

import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryKey
} from "@tanstack/react-query";
import { useCallback, useState } from "react";

import type { ApiResult } from "@/lib/api/client";
export { useClientReady } from "@/lib/react/use-client-ready";

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
  invalidateQueryKey?: QueryKey;
  invalidateQueryKeys?: QueryKey[];
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

export function settingsErrorMessage(
  error: unknown,
  fallbackMessage = "Request failed.",
): string {
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message;
  }
  return fallbackMessage;
}

export function settingsReadPermissionNotice({
  canRead,
  errorStatus,
  isChecking,
  message,
}: {
  canRead: boolean;
  errorStatus?: number;
  isChecking: boolean;
  message: string;
}): SettingsNotice | null {
  if (canRead) {
    return null;
  }
  if (errorStatus !== 403 && isChecking) {
    return null;
  }
  return {
    kind: "warning",
    message,
  };
}

export function settingsReadPermissionEmptyDescription({
  canRead,
  deniedDescription,
  emptyDescription,
}: {
  canRead: boolean;
  deniedDescription: string;
  emptyDescription: string;
}): string {
  if (canRead) {
    return emptyDescription;
  }
  return deniedDescription;
}

export function settingsManagePermissionNotice({
  canManage,
  isChecking,
  message,
}: {
  canManage: boolean;
  isChecking: boolean;
  message: string;
}): SettingsNotice | null {
  if (canManage || isChecking) {
    return null;
  }
  return {
    kind: "warning",
    message,
  };
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
  const initialErrorStatus = initialResult.ok
    ? undefined
    : initialResult.error.status;
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
    errorStatus: query.error?.status ?? initialErrorStatus,
    items: query.data === undefined ? [] : selectItems(query.data),
    notice,
    query,
    refresh,
    setNotice
  };
}

export function useSettingsMutation<TVariables, TData>({
  invalidateQueryKey,
  invalidateQueryKeys,
  mutationFn
}: SettingsMutationOptions<TVariables, TData>) {
  const queryClient = useQueryClient();
  const queryKeys =
    invalidateQueryKeys ?? (invalidateQueryKey === undefined ? [] : [invalidateQueryKey]);
  return useMutation<TData, SettingsApiResultError, TVariables>({
    mutationFn: (variables) => unwrapApiResult(mutationFn(variables)),
    onSuccess: () =>
      Promise.all(
        queryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey }))
      )
  });
}

async function unwrapApiResult<T>(request: Promise<ApiResult<T>>): Promise<T> {
  const result = await request;
  if (!result.ok) {
    throw new SettingsApiResultError(result.error.message, result.error.status);
  }
  return result.data;
}
