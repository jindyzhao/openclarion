import type {
  DiagnosisApprovalAuthority,
  DiagnosisApprovalMode,
  DiagnosisConclusionApproval,
} from "./types";

export type DiagnosisApprovalStatus =
  | "not-started"
  | "pending"
  | "satisfied";

export function diagnosisApprovalModeLabel(
  mode: DiagnosisApprovalMode,
): string {
  return mode === "owner_and_leader" ? "Owner + leader" : "Single operator";
}

export function diagnosisApprovalAuthorityLabel(
  authority: DiagnosisApprovalAuthority,
): string {
  return authority === "leader" ? "Leader" : "Owner";
}

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

export function diagnosisActorApprovalBlockReason({
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
}): string {
  if (approvalInFlight) {
    return "Another conclusion approval is in progress.";
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
    return "Current user has already approved this conclusion.";
  }
  if (actor !== "" && digest !== "" && mode === "owner_and_leader") {
    const authority: DiagnosisApprovalAuthority =
      actor === ownerSubject.trim() ? "owner" : "leader";
    if (approvals.some((approval) => approval.authority === authority)) {
      return `${diagnosisApprovalAuthorityLabel(authority)} approval is already satisfied.`;
    }
  }
  return "";
}
