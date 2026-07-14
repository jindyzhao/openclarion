import { describe, expect, it } from "vitest";

import {
  diagnosisActorApprovalBlockReason,
  diagnosisApprovalAuthorityLabel,
  diagnosisApprovalModeLabel,
  diagnosisPendingApprovalAuthorities,
  diagnosisApprovalStatus,
} from "./approval-state";
import type { DiagnosisConclusionApproval } from "./types";

const approval: DiagnosisConclusionApproval = {
  id: 1,
  conclusion_digest: "a".repeat(64),
  actor_subject: "iam:owner-1",
  authority: "owner",
  reason: "human_confirmed",
  approved_at: "2026-07-14T01:00:00Z",
};

describe("diagnosis approval state", () => {
  it("labels approval modes and authorities", () => {
    expect(diagnosisApprovalModeLabel("single")).toBe("Single operator");
    expect(diagnosisApprovalModeLabel("owner_and_leader")).toBe(
      "Owner + leader",
    );
    expect(diagnosisApprovalAuthorityLabel("owner")).toBe("Owner");
    expect(diagnosisApprovalAuthorityLabel("leader")).toBe("Leader");
  });

  it("derives not-started, pending, and satisfied states", () => {
    expect(
      diagnosisApprovalStatus({
        approvals: [],
        conclusionDigest: undefined,
        pendingAuthorities: [],
      }),
    ).toBe("not-started");
    expect(
      diagnosisApprovalStatus({
        approvals: [approval],
        conclusionDigest: approval.conclusion_digest,
        pendingAuthorities: ["leader"],
      }),
    ).toBe("pending");
    expect(
      diagnosisApprovalStatus({
        approvals: [approval],
        conclusionDigest: approval.conclusion_digest,
        pendingAuthorities: [],
      }),
    ).toBe("satisfied");
  });

  it("reconstructs pending authorities for persisted room summaries", () => {
    expect(
      diagnosisPendingApprovalAuthorities({
        approvals: [],
        conclusionDigest: approval.conclusion_digest,
        mode: "single",
      }),
    ).toEqual(["owner"]);
    expect(
      diagnosisPendingApprovalAuthorities({
        approvals: [approval],
        conclusionDigest: approval.conclusion_digest,
        mode: "owner_and_leader",
      }),
    ).toEqual(["leader"]);
  });

  it("blocks concurrent and duplicate actor approvals", () => {
    expect(
      diagnosisActorApprovalBlockReason({
        actorSubject: "iam:leader-1",
        approvalInFlight: true,
        approvals: [],
        conclusionDigest: approval.conclusion_digest,
        mode: "owner_and_leader",
        ownerSubject: "iam:owner-1",
      }),
    ).toBe("Another conclusion approval is in progress.");
    expect(
      diagnosisActorApprovalBlockReason({
        actorSubject: " iam:owner-1 ",
        approvalInFlight: false,
        approvals: [approval],
        conclusionDigest: approval.conclusion_digest,
        mode: "owner_and_leader",
        ownerSubject: "iam:owner-1",
      }),
    ).toBe("Current user has already approved this conclusion.");
    expect(
      diagnosisActorApprovalBlockReason({
        actorSubject: "iam:leader-1",
        approvalInFlight: false,
        approvals: [approval],
        conclusionDigest: approval.conclusion_digest,
        mode: "owner_and_leader",
        ownerSubject: "iam:owner-1",
      }),
    ).toBe("");
    expect(
      diagnosisActorApprovalBlockReason({
        actorSubject: "iam:leader-2",
        approvalInFlight: false,
        approvals: [
          {
            ...approval,
            actor_subject: "iam:leader-1",
            authority: "leader",
          },
        ],
        conclusionDigest: approval.conclusion_digest,
        mode: "owner_and_leader",
        ownerSubject: "iam:owner-1",
      }),
    ).toBe("Leader approval is already satisfied.");
  });
});
