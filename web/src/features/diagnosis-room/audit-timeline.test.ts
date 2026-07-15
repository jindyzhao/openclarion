import { describe, expect, it } from "vitest";

import type { DiagnosisRoomAuditTimelineEntry } from "./api";
import {
  diagnosisAuditCategoryTranslationKey,
  diagnosisAuditEventTranslationKey,
  diagnosisAuditTimelineColor,
  diagnosisAuditTimelineEntryKey,
} from "./audit-timeline";

describe("diagnosis audit timeline", () => {
  it("maps known event kinds and preserves an explicit unknown fallback", () => {
    expect(
      diagnosisAuditEventTranslationKey("diagnosis_room.opened"),
    ).toBe("auditEventOpened");
    expect(diagnosisAuditEventTranslationKey("custom.event")).toBeNull();
  });

  it("maps every category to a catalog key and stable color", () => {
    expect(diagnosisAuditCategoryTranslationKey("evidence")).toBe(
      "auditCategoryEvidence",
    );
    expect(diagnosisAuditTimelineColor("approval")).toBe("gold");
    expect(diagnosisAuditTimelineColor("other")).toBe("gray");
  });

  it("uses immutable event identity for keys", () => {
    const entry = {
      id: 42,
      event_kind: "diagnosis_room.turn_persisted",
      category: "interaction",
      occurred_at: "2026-07-12T02:00:00Z",
      recorded_at: "2026-07-12T02:00:01Z",
    } satisfies DiagnosisRoomAuditTimelineEntry;

    expect(diagnosisAuditTimelineEntryKey(entry)).toBe(
      "42:diagnosis_room.turn_persisted",
    );
  });
});
