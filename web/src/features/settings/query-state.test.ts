import { describe, expect, it } from "vitest";

import {
  settingsErrorMessage,
  settingsManagePermissionNotice,
  settingsReadPermissionEmptyDescription,
  settingsReadPermissionNotice,
} from "./query-state";

describe("settings query state helpers", () => {
  it("uses a localized fallback only when no useful error message exists", () => {
    expect(settingsErrorMessage(new Error("Backend rejected the request."), "请求失败。"))
      .toBe("Backend rejected the request.");
    expect(settingsErrorMessage(new Error("   "), "请求失败。"))
      .toBe("请求失败。");
    expect(settingsErrorMessage(null, "请求失败。"))
      .toBe("请求失败。");
  });

  it("does not warn while read authorization is still checking", () => {
    expect(
      settingsReadPermissionNotice({
        canRead: false,
        isChecking: true,
        message: "Read access is limited for alert sources.",
      }),
    ).toBeNull();
  });

  it("warns when read access is denied by current authorization", () => {
    expect(
      settingsReadPermissionNotice({
        canRead: false,
        isChecking: false,
        message:
          "Read access is limited for alert sources. Ask an OpenClarion administrator for the matching read role or scoped assignment.",
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
        message: "Read access is limited for notification channels.",
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
        message: "unused",
      }),
    ).toBeNull();
  });

  it("separates configured-empty and permission-limited table empty text", () => {
    expect(
      settingsReadPermissionEmptyDescription({
        canRead: true,
        deniedDescription: "No read access to alert sources.",
        emptyDescription: "No alert sources configured.",
      }),
    ).toBe("No alert sources configured.");

    expect(
      settingsReadPermissionEmptyDescription({
        canRead: false,
        deniedDescription: "No read access to alert sources.",
        emptyDescription: "No alert sources configured.",
      }),
    ).toBe("No read access to alert sources.");
  });

  it("explains read-only forms after manage authorization resolves", () => {
    expect(
      settingsManagePermissionNotice({
        canManage: false,
        isChecking: true,
        message: "This form is read-only for alert source creation.",
      }),
    ).toBeNull();

    expect(
      settingsManagePermissionNotice({
        canManage: true,
        isChecking: false,
        message: "unused",
      }),
    ).toBeNull();

    expect(
      settingsManagePermissionNotice({
        canManage: false,
        isChecking: false,
        message:
          "This form is read-only for alert source creation. Ask an OpenClarion administrator for the matching manage role or scoped assignment.",
      }),
    ).toEqual({
      kind: "warning",
      message:
        "This form is read-only for alert source creation. Ask an OpenClarion administrator for the matching manage role or scoped assignment.",
    });
  });
});
