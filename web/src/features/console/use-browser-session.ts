"use client";

import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useRouter } from "next/navigation";

import {
  clearDiagnosisBrowserSession,
  fetchDiagnosisAuthStatus,
  fetchAccessibleTenants,
  fetchDiagnosisBrowserSession,
  switchDiagnosisBrowserTenant,
  type DiagnosisBrowserSessionStatus,
} from "@/features/diagnosis-room/transport";
import { diagnosisRoomAuthStatusQueryKey } from "@/features/diagnosis-room/cache";

import {
  clearConsoleQueryCacheAfterSignOut,
  consoleBrowserSessionQueryKey,
  replaceConsoleQueryCacheAfterAuthentication,
} from "./session-state";

export const accessibleTenantsQueryKey = ["console", "tenants"] as const;
type AuthenticatedConsoleSession = Extract<
  DiagnosisBrowserSessionStatus,
  { authenticated: true }
>;

export class ConsoleSessionRequestError extends Error {
  readonly status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = "ConsoleSessionRequestError";
    this.status = status;
  }
}

export function useAccessibleTenantsQuery(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: accessibleTenantsQueryKey,
    queryFn: fetchAccessibleTenants,
    retry: false,
    staleTime: 30_000,
  });
}

export function useConsoleAuthStatusQuery(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: diagnosisRoomAuthStatusQueryKey,
    queryFn: fetchDiagnosisAuthStatus,
    retry: false,
    staleTime: 30_000,
  });
}

export function useSwitchConsoleTenant() {
  const queryClient = useQueryClient();
  const router = useRouter();

  return useMutation<
    AuthenticatedConsoleSession,
    ConsoleSessionRequestError,
    string
  >({
    mutationFn: async (tenantKey) => {
      const result = await switchDiagnosisBrowserTenant(tenantKey);
      if (!result.ok) {
        throw new ConsoleSessionRequestError(
          result.error.message,
          result.error.status,
        );
      }
      if (!result.data.authenticated) {
        throw new ConsoleSessionRequestError(
          "Tenant switch did not return an authenticated session.",
          502,
        );
      }
      return result.data;
    },
    onSuccess: async (session) => {
      await replaceConsoleQueryCacheAfterAuthentication(queryClient, session);
      router.refresh();
    },
  });
}

export function useConsoleBrowserSessionQuery() {
  return useQuery({
    queryKey: consoleBrowserSessionQueryKey,
    queryFn: fetchDiagnosisBrowserSession,
    refetchOnWindowFocus: true,
    retry: false,
    staleTime: 30_000,
  });
}

export function useClearConsoleBrowserSession() {
  const queryClient = useQueryClient();
  const router = useRouter();

  return useMutation<void, ConsoleSessionRequestError, void>({
    mutationFn: async () => {
      const result = await clearDiagnosisBrowserSession();
      if (!result.ok) {
        throw new ConsoleSessionRequestError(
          result.error.message,
          result.error.status,
        );
      }
    },
    onSuccess: async () => {
      await clearConsoleQueryCacheAfterSignOut(queryClient);
      router.refresh();
    },
  });
}
