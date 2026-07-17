import type {
  DiagnosisApprovalAuthority,
  DiagnosisApprovalMode,
  DiagnosisConclusionApproval,
} from "./types";

export type DiagnosisApprovalStatus =
  | "not-started"
  | "pending"
  | "satisfied";

export type DiagnosisActorApprovalBlocker =
  | { authority: DiagnosisApprovalAuthority; kind: "authority_satisfied" }
  | { kind: "approval_in_flight" }
  | { kind: "already_approved" };

export function diagnosisApprovalStatus({
  approvals,
  conclusionDigest,
  pendingAuthorities,
}: {
  approvals: readonly DiagnosisConclusionApproval[];
  conclusionDigest: string | undefined;
  pendingAuthorities: readonly DiagnosisApprovalAuthority[];
}): DiagnosisApprovalStatus {
  if (conclusionDigest === undefined || conclusionDigest === "") {
    return "not-started";
  }
  if (pendingAuthorities.length > 0) {
    return "pending";
  }
  return approvals.length > 0 ? "satisfied" : "not-started";
}

export function diagnosisPendingApprovalAuthorities({
  approvals,
  conclusionDigest,
  mode,
}: {
  approvals: readonly DiagnosisConclusionApproval[];
  conclusionDigest: string | undefined;
  mode: DiagnosisApprovalMode;
}): DiagnosisApprovalAuthority[] {
  if (conclusionDigest === undefined || conclusionDigest === "") {
    return [];
  }
  if (mode === "single") {
    return approvals.length > 0 ? [] : ["owner"];
  }
  const approved = new Set(approvals.map((approval) => approval.authority));
  return (["owner", "leader"] as const).filter(
    (authority) => !approved.has(authority),
  );
}

export function diagnosisActorApprovalBlocker({
  actorSubject,
  approvalInFlight,
  approvals,
  conclusionDigest,
  mode,
  ownerSubject,
}: {
  actorSubject: string;
  approvalInFlight: boolean;
  approvals: readonly DiagnosisConclusionApproval[];
  conclusionDigest: string | undefined;
  mode: DiagnosisApprovalMode;
  ownerSubject: string;
}): DiagnosisActorApprovalBlocker | null {
  if (approvalInFlight) {
    return { kind: "approval_in_flight" };
  }
  const actor = actorSubject.trim();
  const digest = conclusionDigest?.trim() ?? "";
  if (
    actor !== "" &&
    digest !== "" &&
    approvals.some(
      (approval) =>
        approval.actor_subject === actor &&
        approval.conclusion_digest === digest,
    )
  ) {
    return { kind: "already_approved" };
  }
  if (actor !== "" && digest !== "" && mode === "owner_and_leader") {
    const authority: DiagnosisApprovalAuthority =
      actor === ownerSubject.trim() ? "owner" : "leader";
    if (approvals.some((approval) => approval.authority === authority)) {
      return { authority, kind: "authority_satisfied" };
    }
  }
  return null;
}
