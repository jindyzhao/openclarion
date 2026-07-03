import { afterEach, describe, expect, it, vi } from "vitest";

import {
  checkCurrentRBACAuthorizations,
  previewRBACAuthorization,
  refreshDirectorySyncRuns,
  refreshRBACAssignments,
  runDirectorySync,
  submitRBACAssignment,
} from "./client-api";

describe("directory and rbac client API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("posts directory sync requests to the same-origin route", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              department_pages: 1,
              user_pages: 1,
              departments_upserted: 2,
              users_upserted: 3,
              users_deactivated: 1,
              synced_at: "2026-06-26T08:00:00Z",
            }),
            { headers: { "content-type": "application/json" }, status: 200 },
          ),
      ),
    );

    await expect(runDirectorySync({ page_size: 100 })).resolves.toMatchObject({
      ok: true,
      data: { users_deactivated: 1, users_upserted: 3 },
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/config/directory/sync",
      expect.objectContaining({
        body: JSON.stringify({ page_size: 100 }),
        cache: "no-store",
        method: "POST",
      }),
    );
  });

  it("uses same-origin rbac routes for assignment and authorization actions", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (
        url === "/api/config/rbac/assignments?limit=100" ||
        url === "/api/config/directory/sync-runs?limit=10"
      ) {
        return new Response(JSON.stringify({ items: [] }), {
          headers: { "content-type": "application/json" },
          status: 200,
        });
      }
      if (url === "/api/config/rbac/assignments") {
        return new Response(
          JSON.stringify({
            created_at: "2026-06-26T08:00:00Z",
            enabled: true,
            id: 1,
            role: "responder",
            scope_key: "room-1",
            scope_kind: "diagnosis_room",
            subject_key: "dep-2",
            subject_kind: "department",
            updated_at: "2026-06-26T08:00:00Z",
          }),
          { headers: { "content-type": "application/json" }, status: 200 },
        );
      }
      if (url === "/api/config/rbac/current-authorizations") {
        return new Response(
          JSON.stringify({
            decisions: [
              {
                allowed: true,
                checked_at: "2026-06-26T08:00:00Z",
                permission: "directory.read",
                scope_key: "",
                scope_kind: "global",
              },
            ],
            department_keys: ["dep-2"],
            directory_users: [],
            subject: "iam-user-1",
          }),
          { headers: { "content-type": "application/json" }, status: 200 },
        );
      }
      return new Response(
        JSON.stringify({
          allowed: true,
          checked_at: "2026-06-26T08:00:00Z",
        }),
        { headers: { "content-type": "application/json" }, status: 200 },
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(refreshRBACAssignments()).resolves.toMatchObject({
      ok: true,
      data: { items: [] },
    });
    await expect(refreshDirectorySyncRuns()).resolves.toMatchObject({
      ok: true,
      data: { items: [] },
    });
    await expect(
      submitRBACAssignment({
        enabled: true,
        role: "responder",
        scope_key: "room-1",
        scope_kind: "diagnosis_room",
        subject_key: "dep-2",
        subject_kind: "department",
      }),
    ).resolves.toMatchObject({
      ok: true,
      data: { id: 1 },
    });
    await expect(
      previewRBACAuthorization({
        department_keys: ["dep-2"],
        permission: "diagnosis_room.participate",
        scope_key: "room-1",
        scope_kind: "diagnosis_room",
        subject: "iam-user-1",
      }),
    ).resolves.toMatchObject({
      ok: true,
      data: { allowed: true },
    });
    await expect(
      checkCurrentRBACAuthorizations({
        requests: [
          { permission: "directory.read", scope_key: "", scope_kind: "global" },
        ],
      }),
    ).resolves.toMatchObject({
      ok: true,
      data: { decisions: [{ allowed: true }], subject: "iam-user-1" },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/config/rbac/assignments?limit=100",
      expect.objectContaining({ cache: "no-store", method: "GET" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/config/rbac/assignments",
      expect.objectContaining({ cache: "no-store", method: "POST" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/config/rbac/authorize",
      expect.objectContaining({ cache: "no-store", method: "POST" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/config/rbac/current-authorizations",
      expect.objectContaining({ cache: "no-store", method: "POST" }),
    );
  });
});
