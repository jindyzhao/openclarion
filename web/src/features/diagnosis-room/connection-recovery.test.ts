import { describe, expect, it } from "vitest";

import {
  diagnosisConnectionTargetSessionID,
  diagnosisReconnectDecision,
} from "./connection-recovery";

describe("diagnosisConnectionTargetSessionID", () => {
  it("prefers selected room summary over selected and form sessions", () => {
    expect(
      diagnosisConnectionTargetSessionID({
        formSessionID: "form-session",
        selectedRoomSessionID: " selected-room ",
        selectedSessionID: "selected-session",
      }),
    ).toBe("selected-room");
  });

  it("falls back to selected session and then form session", () => {
    expect(
      diagnosisConnectionTargetSessionID({
        formSessionID: "form-session",
        selectedSessionID: " selected-session ",
      }),
    ).toBe("selected-session");

    expect(
      diagnosisConnectionTargetSessionID({
        formSessionID: " form-session ",
        selectedSessionID: " ",
      }),
    ).toBe("form-session");
  });

  it("returns blank when no connection target is available", () => {
    expect(
      diagnosisConnectionTargetSessionID({
        formSessionID: " ",
        selectedRoomSessionID: "",
        selectedSessionID: undefined,
      }),
    ).toBe("");
  });
});

describe("diagnosisReconnectDecision", () => {
  const connection = {
    authorization: { mode: "bearer" as const, token: " token " },
    sessionID: " session-1 ",
  };

  it("schedules the next reconnect with normalized credentials", () => {
    expect(
      diagnosisReconnectDecision({
        attempts: 1,
        clientReady: true,
        connection,
        maxAttempts: 3,
        manualDisconnect: false,
        timerActive: false,
      }),
    ).toEqual({
      kind: "schedule",
      attempt: 2,
      connection: {
        authorization: { mode: "bearer", token: "token" },
        sessionID: "session-1",
      },
    });
  });

  it("does not schedule reconnect after a manual disconnect", () => {
    expect(
      diagnosisReconnectDecision({
        attempts: 0,
        clientReady: true,
        connection,
        maxAttempts: 3,
        manualDisconnect: true,
        timerActive: false,
      }),
    ).toEqual({ kind: "skip", reason: "manual_disconnect" });
  });

  it("does not schedule reconnect before the client is ready", () => {
    expect(
      diagnosisReconnectDecision({
        attempts: 0,
        clientReady: false,
        connection,
        maxAttempts: 3,
        manualDisconnect: false,
        timerActive: false,
      }),
    ).toEqual({ kind: "skip", reason: "client_not_ready" });
  });

  it("rejects missing or blank reconnect credentials", () => {
    expect(
      diagnosisReconnectDecision({
        attempts: 0,
        clientReady: true,
        connection: {
          authorization: { mode: "bearer", token: " " },
          sessionID: "session-1",
        },
        maxAttempts: 3,
        manualDisconnect: false,
        timerActive: false,
      }),
    ).toEqual({ kind: "skip", reason: "missing_connection" });
  });

  it("does not schedule a duplicate timer", () => {
    expect(
      diagnosisReconnectDecision({
        attempts: 0,
        clientReady: true,
        connection,
        maxAttempts: 3,
        manualDisconnect: false,
        timerActive: true,
      }),
    ).toEqual({ kind: "already_scheduled" });
  });

  it("stops after the configured attempt limit", () => {
    expect(
      diagnosisReconnectDecision({
        attempts: 3,
        clientReady: true,
        connection,
        maxAttempts: 3,
        manualDisconnect: false,
        timerActive: false,
      }),
    ).toEqual({ kind: "exhausted" });
  });
});
