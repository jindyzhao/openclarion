"use client";

import { useEffect, useMemo, useState } from "react";

import {
  checkCurrentRBACAuthorizations,
} from "./directory-rbac/client-api";
import type {
  DirectoryUser,
  RBACCurrentAuthorizationResponse,
  RBACPermission,
  RBACScopeKind,
} from "./directory-rbac/types";
import type { SettingsNotice } from "./query-state";

const currentRBACAuthorizationBatchLimit = 50;

export type CurrentRBACAuthorizationCheck = {
  key: string;
  permission: RBACPermission;
  scopeKey?: string;
  scopeKind?: RBACScopeKind;
};

export type CurrentRBACAuthorizationState =
  | { fingerprint: string; kind: "loading" }
  | {
      allowed: Record<string, boolean>;
      departmentKeys: string[];
      directoryUsers: DirectoryUser[];
      fingerprint: string;
      kind: "ready";
      subject: string;
    }
  | { fingerprint: string; kind: "error"; message: string; status?: number };

export type CurrentRBACAuthorizations = {
  can: (key: string) => boolean;
  isChecking: boolean;
  notice: SettingsNotice | null;
  state: CurrentRBACAuthorizationState;
};

export function useCurrentRBACAuthorizations(
  checks: readonly CurrentRBACAuthorizationCheck[],
  enabled: boolean,
): CurrentRBACAuthorizations {
  const [state, setState] = useState<CurrentRBACAuthorizationState>({
    fingerprint: "",
    kind: "loading",
  });
  const fingerprint = useMemo(() => authorizationChecksFingerprint(checks), [
    checks,
  ]);
  const active = enabled && checks.length > 0;

  useEffect(() => {
    if (!active) {
      return;
    }
    let ignore = false;
    void checkCurrentRBACAuthorizationBatches(checks).then((result) => {
      if (ignore) {
        return;
      }
      if (!result.ok) {
        setState({
          fingerprint,
          kind: "error",
          message: result.error.message,
          status: result.error.status,
        });
        return;
      }
      setState(
        currentRBACAuthorizationStateFromResponse(
          checks,
          fingerprint,
          result.data,
        ),
      );
    });
    return () => {
      ignore = true;
    };
  }, [checks, active, fingerprint]);

  const effectiveState = useMemo(
    () =>
      currentRBACAuthorizationStateAfterActivationChange({
        active,
        current: state,
        fingerprint,
      }),
    [active, fingerprint, state],
  );
  const current = effectiveState.fingerprint === fingerprint;

  return useMemo(() => {
    const view = currentRBACAuthorizationsFromState({
      active,
      current,
      state: effectiveState,
    });
    return { ...view, state: effectiveState };
  }, [active, current, effectiveState]);
}

async function checkCurrentRBACAuthorizationBatches(
  checks: readonly CurrentRBACAuthorizationCheck[],
) {
  const batches = currentRBACAuthorizationCheckBatches(checks);
  const responses = await Promise.all(
    batches.map((batch) =>
      checkCurrentRBACAuthorizations({
        requests: batch.map((check) => ({
          permission: check.permission,
          scope_key: check.scopeKey ?? "",
          scope_kind: check.scopeKind ?? "global",
        })),
      }),
    ),
  );
  const failed = responses.find((response) => !response.ok);
  if (failed !== undefined && !failed.ok) {
    return failed;
  }
  const successful = responses.filter((response) => response.ok);
  const [first] = successful;
  if (first === undefined) {
    return {
      ok: true as const,
      data: {
        decisions: [],
        department_keys: [],
        directory_users: [],
        subject: "",
      },
    };
  }
  return {
    ok: true as const,
    data: {
      decisions: successful.flatMap((response) => response.data.decisions),
      department_keys: first.data.department_keys,
      directory_users: first.data.directory_users,
      subject: first.data.subject,
    },
  };
}

export function currentRBACAuthorizationCheckBatches(
  checks: readonly CurrentRBACAuthorizationCheck[],
  batchLimit = currentRBACAuthorizationBatchLimit,
): CurrentRBACAuthorizationCheck[][] {
  const limit = Math.max(1, Math.floor(batchLimit));
  const batches: CurrentRBACAuthorizationCheck[][] = [];
  for (let index = 0; index < checks.length; index += limit) {
    batches.push([...checks.slice(index, index + limit)]);
  }
  return batches;
}

export function currentRBACAuthorizationsFromState({
  active,
  current,
  state,
}: {
  active: boolean;
  current: boolean;
  state: CurrentRBACAuthorizationState;
}): Omit<CurrentRBACAuthorizations, "state"> {
  return {
    can: (key: string) =>
      active && current && state.kind === "ready" && state.allowed[key] === true,
    isChecking: active && (!current || state.kind === "loading"),
    notice:
      active && current && state.kind === "error"
        ? {
            kind: "warning",
            message: `Current authorization check failed: ${state.message}`,
          }
        : null,
  };
}

export function currentRBACAuthorizationStateAfterActivationChange({
  active,
  current,
  fingerprint,
}: {
  active: boolean;
  current: CurrentRBACAuthorizationState;
  fingerprint: string;
}): CurrentRBACAuthorizationState {
  if (!active) {
    return current.kind === "loading" && current.fingerprint === fingerprint
      ? current
      : { fingerprint, kind: "loading" };
  }
  if (current.fingerprint !== fingerprint) {
    return { fingerprint, kind: "loading" };
  }
  return current;
}

export function currentRBACAuthorizationNeedsSignIn(
  state: CurrentRBACAuthorizationState,
): boolean {
  return state.kind === "error" && state.status === 401;
}

function currentRBACAuthorizationStateFromResponse(
  checks: readonly CurrentRBACAuthorizationCheck[],
  fingerprint: string,
  response: RBACCurrentAuthorizationResponse,
): CurrentRBACAuthorizationState {
  const allowed: Record<string, boolean> = {};
  checks.forEach((check, index) => {
    allowed[check.key] = response.decisions[index]?.allowed === true;
  });
  return {
    allowed,
    departmentKeys: response.department_keys,
    directoryUsers: response.directory_users,
    fingerprint,
    kind: "ready",
    subject: response.subject,
  };
}

function authorizationChecksFingerprint(
  checks: readonly CurrentRBACAuthorizationCheck[],
): string {
  return checks
    .map((check) =>
      [
        check.key,
        check.permission,
        check.scopeKind ?? "global",
        check.scopeKey ?? "",
      ].join("\u001f"),
    )
    .join("\u001e");
}
