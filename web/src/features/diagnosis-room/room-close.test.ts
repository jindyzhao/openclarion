import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";
import { diagnosisCloseRoomBlockReason } from "./room-close";

const openState = { status: "open" };
const tEn = createTranslator({ locale: "en", messages: en, namespace: "DiagnosisRoom" });
const tZhCN = createTranslator({ locale: "zh-CN", messages: zhCN, namespace: "DiagnosisRoom" });

function roomCloseMessages(
  t: typeof tEn,
): Parameters<typeof diagnosisCloseRoomBlockReason>[0]["messages"] {
  return {
    alreadyClosed: t("closeAlreadyClosed"),
    closeProcessing: t("closeRequestProcessing"),
    connectRequired: t("closeConnectRequired"),
    identityRequired: t("closeIdentityRequired"),
    refreshRequired: t("closeRefreshRequired"),
    waitForConfirmation: t("closeWaitForConfirmation"),
    waitForTurn: t("closeWaitForTurn"),
  };
}

describe("diagnosis room close readiness", () => {
  it("allows an authorized connected operator to close an idle open room", () => {
    expect(
      diagnosisCloseRoomBlockReason({
        actorSubject: "operator-1",
        closeInFlight: false,
        confirmInFlight: false,
        connected: true,
        messages: roomCloseMessages(tEn),
        rbacBlockReason: "",
        state: openState,
        turnInFlight: false,
      }),
    ).toBe("");
  });

  it.each([
    {
      name: "disconnected",
      input: { connected: false },
      want: "Connect to the diagnosis room before closing it.",
    },
    {
      name: "state unavailable",
      input: { state: null },
      want: "Refresh the room state before closing it.",
    },
    {
      name: "turn in flight",
      input: { turnInFlight: true },
      want: "Wait for the current diagnosis turn to finish before closing the room.",
    },
    {
      name: "already closed",
      input: { state: { status: "closed" } },
      want: "This diagnosis room is already closed.",
    },
    {
      name: "missing identity",
      input: { actorSubject: " " },
      want: "An authenticated operator identity is required to close the room.",
    },
    {
      name: "permission denied",
      input: { rbacBlockReason: "Administer access is required." },
      want: "Administer access is required.",
    },
  ])("blocks $name", ({ input, want }) => {
    expect(
      diagnosisCloseRoomBlockReason({
        actorSubject: "operator-1",
        closeInFlight: false,
        confirmInFlight: false,
        connected: true,
        messages: roomCloseMessages(tEn),
        rbacBlockReason: "",
        state: openState,
        turnInFlight: false,
        ...input,
      }),
    ).toBe(want);
  });

  it("prioritizes an existing close request over other blockers", () => {
    expect(
      diagnosisCloseRoomBlockReason({
        actorSubject: "",
        closeInFlight: true,
        confirmInFlight: true,
        connected: false,
        messages: roomCloseMessages(tEn),
        rbacBlockReason: "Administer access is required.",
        state: null,
        turnInFlight: true,
      }),
    ).toBe("The diagnosis room close request is still processing.");
  });

  it("returns Chinese operator guidance", () => {
    expect(
      diagnosisCloseRoomBlockReason({
        actorSubject: "operator-1",
        closeInFlight: false,
        confirmInFlight: false,
        connected: true,
        messages: roomCloseMessages(tZhCN),
        rbacBlockReason: "",
        state: openState,
        turnInFlight: true,
      }),
    ).toBe("当前诊断轮次完成后才能关闭诊断室。");
  });
});
