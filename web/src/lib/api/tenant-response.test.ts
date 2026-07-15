import { describe, expect, it } from "vitest";

import {
  normalizedTenantListResponse,
  normalizedTenantMembershipListResponse,
} from "./tenant-response";

describe("normalizedTenantListResponse", () => {
  it("accepts unique bounded tenant rows", () => {
    expect(
      normalizedTenantListResponse({
        items: [
          {
            id: 1,
            key: "default",
            name: "Default",
            status: "active",
            created_at: "2026-07-11T10:00:00Z",
            updated_at: "2026-07-11T10:00:00Z",
          },
        ],
      }),
    ).toEqual({
      items: [
        expect.objectContaining({ id: 1, key: "default", status: "active" }),
      ],
    });
  });

  it("rejects malformed and duplicate tenant identities", () => {
    expect(normalizedTenantListResponse({ items: [{ id: 0 }] })).toBeNull();
    const row = {
      id: 1,
      key: "default",
      name: "Default",
      status: "active",
      created_at: "2026-07-11T10:00:00Z",
      updated_at: "2026-07-11T10:00:00Z",
    };
    expect(normalizedTenantListResponse({ items: [row, row] })).toBeNull();
    expect(
      normalizedTenantListResponse({
        items: [{ ...row, created_at: "2026-07-11" }],
      }),
    ).toBeNull();
    expect(
      normalizedTenantListResponse({
        items: [{ ...row, created_at: "2026-02-30T10:00:00Z" }],
      }),
    ).toBeNull();
  });

  it("uses Unicode code points for workspace name limits", () => {
    const row = {
      id: 1,
      key: "default",
      status: "active",
      created_at: "2026-07-11T10:00:00Z",
      updated_at: "2026-07-11T10:00:00Z",
    };
    expect(
      normalizedTenantListResponse({
        items: [{ ...row, name: "诊".repeat(120) }],
      }),
    ).not.toBeNull();
    expect(
      normalizedTenantListResponse({
        items: [{ ...row, name: "诊".repeat(121) }],
      }),
    ).toBeNull();
  });
});

describe("normalizedTenantMembershipListResponse", () => {
  it("accepts a unique, bounded membership list", () => {
    expect(
      normalizedTenantMembershipListResponse(
        {
          items: [
            {
              id: 3,
              tenant_id: 2,
              subject: "operator-1",
              role: "owner",
              enabled: true,
              created_by: "bootstrap-1",
              created_at: "2026-07-11T10:00:00Z",
              updated_at: "2026-07-11T10:00:00Z",
            },
          ],
        },
        2,
      ),
    ).toMatchObject({ items: [{ id: 3, subject: "operator-1" }] });
  });

  it("rejects duplicate subjects and malformed membership data", () => {
    const membership = {
      id: 3,
      tenant_id: 2,
      subject: "operator-1",
      role: "owner",
      enabled: true,
      created_by: "bootstrap-1",
      created_at: "2026-07-11T10:00:00Z",
      updated_at: "2026-07-11T10:00:00Z",
    };
    expect(
      normalizedTenantMembershipListResponse(
        { items: [membership, { ...membership, id: 4 }] },
        2,
      ),
    ).toBeNull();
    expect(
      normalizedTenantMembershipListResponse(
        { items: [{ ...membership, role: "admin" }] },
        2,
      ),
    ).toBeNull();
  });

  it("rejects membership rows from another tenant", () => {
    expect(
      normalizedTenantMembershipListResponse(
        {
          items: [
            {
              id: 3,
              tenant_id: 7,
              subject: "operator-1",
              role: "owner",
              enabled: true,
              created_by: "bootstrap-1",
              created_at: "2026-07-11T10:00:00Z",
              updated_at: "2026-07-11T10:00:00Z",
            },
          ],
        },
        2,
      ),
    ).toBeNull();
  });
});
