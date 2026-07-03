import { describe, expect, it } from "vitest";

import {
  diagnosisHandoffListQueryKey,
  diagnosisRoomDetailQueryKey,
  diagnosisRoomListQueryKey,
  diagnosisRoomRefreshQueryKeys,
} from "./cache";

describe("diagnosis room cache keys", () => {
  it("builds refresh keys for room list, handoffs, and an exact room", () => {
    expect(diagnosisRoomRefreshQueryKeys(" diagnosis-session-1 ")).toEqual([
      diagnosisRoomListQueryKey,
      diagnosisHandoffListQueryKey,
      diagnosisRoomDetailQueryKey("diagnosis-session-1"),
    ]);
  });

  it("omits exact room refresh when no session is known", () => {
    expect(diagnosisRoomRefreshQueryKeys(" ")).toEqual([
      diagnosisRoomListQueryKey,
      diagnosisHandoffListQueryKey,
    ]);
  });
});
