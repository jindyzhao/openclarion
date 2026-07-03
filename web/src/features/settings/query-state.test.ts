import { describe, expect, it } from "vitest";

import {
  settingsManagePermissionNotice,
  settingsReadPermissionEmptyDescription,
  settingsReadPermissionNotice,
} from "./query-state";

describe("settings query state helpers", () => {
  it("does not warn while read authorization is still checking", () => {
    expect(
      settingsReadPermissionNotice({
        canRead: false,
        isChecking: true,
        resourceLabel: "alert sources",
      }),
    ).toBeNull();
  });

  it("warns when read access is denied by current authorization", () => {
    expect(
      settingsReadPermissionNotice({
        canRead: false,
        isChecking: false,
        resourceLabel: "alert sources",
      }),
    ).toEqual({
      kind: "warning",
      message:
        "Read access is limited for alert sources. Ask an OpenClarion administrator for the matching read role or scoped assignment.",
    });
  });

  it("warns immediately when the list request returned forbidden", () => {
    expect(
      settingsReadPermissionNotice({
        canRead: false,
        errorStatus: 403,
        isChecking: true,
        resourceLabel: "notification channels",
      }),
    ).toMatchObject({
      kind: "warning",
    });
  });

  it("does not warn when read access is allowed", () => {
    expect(
      settingsReadPermissionNotice({
        canRead: true,
        errorStatus: 403,
        isChecking: false,
        resourceLabel: "alert sources",
      }),
    ).toBeNull();
  });

  it("separates configured-empty and permission-limited table empty text", () => {
    expect(
      settingsReadPermissionEmptyDescription({
        canRead: true,
        emptyDescription: "No alert sources configured.",
        resourceLabel: "alert sources",
      }),
    ).toBe("No alert sources configured.");

    expect(
      settingsReadPermissionEmptyDescription({
        canRead: false,
        emptyDescription: "No alert sources configured.",
        resourceLabel: "alert sources",
      }),
    ).toBe("No read access to alert sources.");
  });

  it("explains read-only forms after manage authorization resolves", () => {
    expect(
      settingsManagePermissionNotice({
        canManage: false,
        isChecking: true,
        resourceLabel: "alert source creation",
      }),
    ).toBeNull();

    expect(
      settingsManagePermissionNotice({
        canManage: true,
        isChecking: false,
        resourceLabel: "alert source creation",
      }),
    ).toBeNull();

    expect(
      settingsManagePermissionNotice({
        canManage: false,
        isChecking: false,
        resourceLabel: "alert source creation",
      }),
    ).toEqual({
      kind: "warning",
      message:
        "This form is read-only for alert source creation. Ask an OpenClarion administrator for the matching manage role or scoped assignment.",
    });
  });
});
