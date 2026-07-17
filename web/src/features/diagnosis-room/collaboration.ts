import type { DiagnosisRoomSummary } from "./api";
import type { DirectoryUser } from "@/features/settings/directory-rbac/types";
import type {
  DiagnosisConversationTurn,
  DiagnosisEvidenceTimelineEntry,
  DiagnosisFinalConclusion,
  DiagnosisSupplementalEvidenceRecord,
} from "./types";

export type DiagnosisCollaborationParticipantRole =
  | "assistant"
  | "confirmation"
  | "evidence"
  | "message"
  | "owner"
  | "supplemental_evidence";

export type DiagnosisCollaborationParticipant = {
  subject: string;
  roles: DiagnosisCollaborationParticipantRole[];
  isSystem: boolean;
  messageCount: number;
  evidenceCollectionCount: number;
  supplementalEvidenceCount: number;
  confirmedConclusion: boolean;
};

export type DiagnosisCollaborationParticipantProfile = {
  displayName: string;
  subject: string;
  detailTags: string[];
  active: boolean | null;
  matchedDirectoryUser: boolean;
};

type DiagnosisCollaborationIdentityCoverageStatus = "empty" | "ready" | "review";

export type DiagnosisCollaborationIdentityCoverage = {
  status: DiagnosisCollaborationIdentityCoverageStatus;
  humanParticipants: number;
  systemActors: number;
  syncedParticipants: number;
  inactiveParticipants: number;
  unsyncedParticipants: number;
};

export type DiagnosisCollaborationDirectoryUser = NonNullable<
  DiagnosisRoomSummary["participant_directory_users"]
>[number];

type DiagnosisCollaborationInput = {
  ownerSubject?: string;
  conversation?: DiagnosisConversationTurn[];
  evidenceTimeline?: DiagnosisEvidenceTimelineEntry[];
  supplementalEvidence?: DiagnosisSupplementalEvidenceRecord[];
  finalConclusion?: DiagnosisFinalConclusion;
};

type MutableParticipant = DiagnosisCollaborationParticipant & {
  roleSet: Set<DiagnosisCollaborationParticipantRole>;
};

type DiagnosisRoomSummaryParticipant = NonNullable<
  DiagnosisRoomSummary["participants"]
>[number];

export function diagnosisCollaborationParticipants({
  ownerSubject,
  conversation = [],
  evidenceTimeline = [],
  supplementalEvidence = [],
  finalConclusion,
}: DiagnosisCollaborationInput): DiagnosisCollaborationParticipant[] {
  const participants = new Map<string, MutableParticipant>();
  const owner = normalizedSubject(ownerSubject);
  if (owner !== "") {
    addParticipantRole(participants, owner, "owner");
  }

  for (const turn of conversation) {
    const subject = normalizedSubject(turn.actor_subject);
    if (subject === "") {
      continue;
    }
    const participant = addParticipantRole(
      participants,
      subject,
      turn.role === "assistant" ? "assistant" : "message",
    );
    participant.messageCount += 1;
  }

  for (const entry of evidenceTimeline) {
    const subject = normalizedSubject(entry.actor_subject);
    if (subject === "") {
      continue;
    }
    const hasEvidence =
      (entry.evidence_requests?.length ?? 0) > 0 ||
      (entry.evidence_collection_results?.length ?? 0) > 0;
    if (!hasEvidence) {
      continue;
    }
    const participant = addParticipantRole(participants, subject, "evidence");
    participant.evidenceCollectionCount += 1;
  }

  for (const item of supplementalEvidence) {
    const subject = normalizedSubject(item.actor_subject);
    if (subject === "") {
      continue;
    }
    const participant = addParticipantRole(
      participants,
      subject,
      "supplemental_evidence",
    );
    participant.supplementalEvidenceCount += 1;
  }

  const confirmedBy = normalizedSubject(finalConclusion?.confirmed_by);
  if (confirmedBy !== "") {
    const participant = addParticipantRole(
      participants,
      confirmedBy,
      "confirmation",
    );
    participant.confirmedConclusion = true;
  }

  return [...participants.values()]
    .map(({ roleSet, ...participant }) => ({
      ...participant,
      roles: [...roleSet].sort(collaborationRoleSort),
    }))
    .sort(collaborationParticipantSort);
}

export function diagnosisCollaborationParticipantsFromSummary(
  participants: DiagnosisRoomSummary["participants"] | undefined,
): DiagnosisCollaborationParticipant[] {
  if (participants === undefined || participants.length === 0) {
    return [];
  }
  return participants
    .map(diagnosisCollaborationParticipantFromSummary)
    .filter(
      (participant): participant is DiagnosisCollaborationParticipant =>
        participant !== null,
    )
    .sort(collaborationParticipantSort);
}

export function diagnosisCollaborationDirectoryIndex(
  users: DiagnosisCollaborationDirectoryUser[],
): ReadonlyMap<string, DiagnosisCollaborationDirectoryUser> {
  const index = new Map<string, DiagnosisCollaborationDirectoryUser>();
  for (const user of users) {
    const subject = normalizedSubject(user.subject);
    if (subject === "" || index.has(subject)) {
      continue;
    }
    index.set(subject, user);
  }
  return index;
}

export function diagnosisCollaborationDirectoryUsersFromRooms(
  rooms: readonly (DiagnosisRoomSummary | undefined)[],
): DiagnosisCollaborationDirectoryUser[] {
  const out: DiagnosisCollaborationDirectoryUser[] = [];
  const seenSubjects = new Set<string>();
  for (const room of rooms) {
    for (const user of room?.participant_directory_users ?? []) {
      const subject = normalizedSubject(user.subject);
      if (subject === "" || seenSubjects.has(subject)) {
        continue;
      }
      seenSubjects.add(subject);
      out.push(user);
    }
  }
  return out;
}

export function diagnosisCollaborationDirectoryUsersFromDirectoryUsers(
  users: readonly DirectoryUser[] | undefined,
): DiagnosisCollaborationDirectoryUser[] {
  const usersBySubject = new Map<string, DiagnosisCollaborationDirectoryUser>();
  for (const user of users ?? []) {
    const projected = diagnosisCollaborationDirectoryUserFromDirectoryUser(user);
    if (projected === null) {
      continue;
    }
    const current = usersBySubject.get(projected.subject);
    if (current === undefined || (!current.active && projected.active)) {
      usersBySubject.set(projected.subject, projected);
    }
  }
  return [...usersBySubject.values()];
}

export function diagnosisCollaborationParticipantProfile(
  participant: DiagnosisCollaborationParticipant,
  directoryUsersBySubject: ReadonlyMap<
    string,
    DiagnosisCollaborationDirectoryUser
  >,
): DiagnosisCollaborationParticipantProfile {
  return diagnosisCollaborationActorProfile(
    participant.subject,
    directoryUsersBySubject,
  );
}

export function diagnosisCollaborationActorProfile(
  subject: string,
  directoryUsersBySubject: ReadonlyMap<
    string,
    DiagnosisCollaborationDirectoryUser
  >,
): DiagnosisCollaborationParticipantProfile {
  const normalized = normalizedSubject(subject);
  const user = directoryUsersBySubject.get(normalized);
  if (user === undefined) {
    return {
      displayName: normalized,
      subject: normalized,
      detailTags: [],
      active: null,
      matchedDirectoryUser: false,
    };
  }
  const displayName = firstNonEmpty(
    user.display_name,
    user.username,
    user.subject,
  );
  const detailTags = uniqueNonEmpty([
    displayName === normalized ? "" : normalized,
    user.department_path,
    ...user.department_paths,
    user.department,
    user.section,
    user.job_title,
  ]);
  return {
    displayName,
    subject: normalized,
    detailTags,
    active: user.active,
    matchedDirectoryUser: true,
  };
}

export function diagnosisCollaborationIdentityCoverage(
  participants: readonly DiagnosisCollaborationParticipant[],
  directoryUsersBySubject: ReadonlyMap<
    string,
    DiagnosisCollaborationDirectoryUser
  >,
): DiagnosisCollaborationIdentityCoverage {
  let systemActors = 0;
  let syncedParticipants = 0;
  let inactiveParticipants = 0;
  let unsyncedParticipants = 0;

  for (const participant of participants) {
    if (participant.isSystem) {
      systemActors += 1;
      continue;
    }
    const profile = diagnosisCollaborationParticipantProfile(
      participant,
      directoryUsersBySubject,
    );
    if (!profile.matchedDirectoryUser) {
      unsyncedParticipants += 1;
      continue;
    }
    if (profile.active === false) {
      inactiveParticipants += 1;
      continue;
    }
    syncedParticipants += 1;
  }

  const humanParticipants =
    syncedParticipants + inactiveParticipants + unsyncedParticipants;
  const status: DiagnosisCollaborationIdentityCoverageStatus =
    humanParticipants === 0
      ? "empty"
      : inactiveParticipants > 0 || unsyncedParticipants > 0
        ? "review"
        : "ready";

  return {
    status,
    humanParticipants,
    systemActors,
    syncedParticipants,
    inactiveParticipants,
    unsyncedParticipants,
  };
}

export function diagnosisCollaborationSubjectIsSystem(subject: string): boolean {
  return isSystemSubject(normalizedSubject(subject));
}

function diagnosisCollaborationParticipantFromSummary(
  participant: DiagnosisRoomSummaryParticipant,
): DiagnosisCollaborationParticipant | null {
  const subject = normalizedSubject(participant.subject);
  if (subject === "") {
    return null;
  }
  return {
    subject,
    roles: participant.roles
      .filter(isDiagnosisCollaborationParticipantRole)
      .sort(collaborationRoleSort),
    isSystem: participant.is_system || isSystemSubject(subject),
    messageCount: nonNegativeInteger(participant.message_count),
    evidenceCollectionCount: nonNegativeInteger(
      participant.evidence_collection_count,
    ),
    supplementalEvidenceCount: nonNegativeInteger(
      participant.supplemental_evidence_count,
    ),
    confirmedConclusion: participant.confirmed_conclusion,
  };
}

function diagnosisCollaborationDirectoryUserFromDirectoryUser(
  user: DirectoryUser,
): DiagnosisCollaborationDirectoryUser | null {
  const subject = normalizedSubject(user.subject);
  if (subject === "") {
    return null;
  }
  return {
    subject,
    username: user.username,
    display_name: user.display_name,
    job_title: user.job_title,
    department: user.department,
    section: user.section,
    department_path: user.department_path,
    department_paths: user.department_paths,
    active: user.active,
  };
}

function addParticipantRole(
  participants: Map<string, MutableParticipant>,
  subject: string,
  role: DiagnosisCollaborationParticipantRole,
): MutableParticipant {
  let participant = participants.get(subject);
  if (participant === undefined) {
    participant = {
      subject,
      roles: [],
      roleSet: new Set<DiagnosisCollaborationParticipantRole>(),
      isSystem: isSystemSubject(subject),
      messageCount: 0,
      evidenceCollectionCount: 0,
      supplementalEvidenceCount: 0,
      confirmedConclusion: false,
    };
    participants.set(subject, participant);
  }
  participant.roleSet.add(role);
  return participant;
}

function collaborationParticipantSort(
  a: DiagnosisCollaborationParticipant,
  b: DiagnosisCollaborationParticipant,
): number {
  if (a.isSystem !== b.isSystem) {
    return a.isSystem ? 1 : -1;
  }
  const activityDelta =
    participantActivityCount(b) - participantActivityCount(a);
  if (activityDelta !== 0) {
    return activityDelta;
  }
  return a.subject.localeCompare(b.subject);
}

function participantActivityCount(
  participant: DiagnosisCollaborationParticipant,
): number {
  return (
    participant.messageCount +
    participant.evidenceCollectionCount +
    participant.supplementalEvidenceCount +
    (participant.confirmedConclusion ? 1 : 0)
  );
}

function collaborationRoleSort(
  a: DiagnosisCollaborationParticipantRole,
  b: DiagnosisCollaborationParticipantRole,
): number {
  return collaborationRoleRank(a) - collaborationRoleRank(b);
}

function collaborationRoleRank(
  role: DiagnosisCollaborationParticipantRole,
): number {
  switch (role) {
    case "owner":
      return 0;
    case "message":
      return 1;
    case "evidence":
      return 2;
    case "supplemental_evidence":
      return 3;
    case "confirmation":
      return 4;
    case "assistant":
      return 5;
  }
}

function normalizedSubject(subject: string | undefined): string {
  return subject?.trim() ?? "";
}

function isDiagnosisCollaborationParticipantRole(
  role: string,
): role is DiagnosisCollaborationParticipantRole {
  switch (role) {
    case "assistant":
    case "confirmation":
    case "evidence":
    case "message":
    case "owner":
    case "supplemental_evidence":
      return true;
    default:
      return false;
  }
}

function nonNegativeInteger(value: number): number {
  if (!Number.isFinite(value) || value <= 0) {
    return 0;
  }
  return Math.trunc(value);
}

function isSystemSubject(subject: string): boolean {
  return (
    subject.startsWith("openclarion:") ||
    subject.startsWith("openclarion.")
  );
}

function firstNonEmpty(...values: string[]): string {
  for (const value of values) {
    const trimmed = value.trim();
    if (trimmed !== "") {
      return trimmed;
    }
  }
  return "";
}

function uniqueNonEmpty(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (trimmed === "" || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}
