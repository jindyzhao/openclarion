import { describe, expect, it } from "vitest";

import type { DirectoryUser } from "@/features/settings/directory-rbac/types";

import {
  diagnosisCollaborationActorProfile,
  diagnosisCollaborationDirectoryIndex,
  diagnosisCollaborationDirectoryUsersFromDirectoryUsers,
  diagnosisCollaborationDirectoryUsersFromRooms,
  diagnosisCollaborationIdentityCoverage,
  diagnosisCollaborationParticipantProfile,
  diagnosisCollaborationParticipants,
  diagnosisCollaborationParticipantsFromSummary,
  diagnosisCollaborationRoleLabel,
  diagnosisCollaborationSubjectIsSystem,
  type DiagnosisCollaborationParticipant,
} from "./collaboration";

describe("diagnosis collaboration helpers", () => {
  it("merges room actors across conversation, evidence, supplemental records, and confirmation", () => {
    const participants = diagnosisCollaborationParticipants({
      ownerSubject: " owner-1 ",
      conversation: [
        { role: "user", actor_subject: "owner-1", content: "Start diagnosis." },
        {
          role: "assistant",
          actor_subject: "openclarion:auto-diagnosis",
          content: "Need evidence.",
        },
        {
          role: "user",
          actor_subject: "reviewer-1",
          content: "I can provide restart context.",
        },
      ],
      evidenceTimeline: [
        {
          turn_count: 1,
          actor_subject: "reviewer-1",
          evidence_requests: [
            { tool: "active_alerts", reason: "Check alerts." },
          ],
        },
        {
          turn_count: 2,
          actor_subject: "openclarion:auto-diagnosis",
          evidence_collection_results: [
            {
              request: { tool: "active_alerts", reason: "Check alerts." },
              tool: "active_alerts",
              status: "collected",
              reason_code: "ok",
              message: "Collected.",
              observed_alerts: 1,
              collected_at: "2026-06-20T00:01:00Z",
            },
          ],
        },
      ],
      supplementalEvidence: [
        {
          label: "Restart cause",
          detail: "Check previous logs.",
          priority: "high",
          evidence: "OOMKilled.",
          actor_subject: "reviewer-1",
          user_message_id: "msg-2",
          assistant_message_id: "msg-2/assistant",
          user_turn_id: 21,
          assistant_turn_id: 22,
          user_sequence: 3,
          assistant_sequence: 4,
          provided_at: "2026-06-20T00:02:00Z",
        },
      ],
      finalConclusion: {
        status: "available",
        source: "operator",
        confirmed_by: "reviewer-1",
      },
    });

    expect(participants).toHaveLength(3);
    expect(participants[0]).toMatchObject({
      subject: "reviewer-1",
      roles: ["message", "evidence", "supplemental_evidence", "confirmation"],
      isSystem: false,
      messageCount: 1,
      evidenceCollectionCount: 1,
      supplementalEvidenceCount: 1,
      confirmedConclusion: true,
    });
    expect(participants[1]).toMatchObject({
      subject: "owner-1",
      roles: ["owner", "message"],
      isSystem: false,
      messageCount: 1,
    });
    expect(participants[2]).toMatchObject({
      subject: "openclarion:auto-diagnosis",
      roles: ["evidence", "assistant"],
      isSystem: true,
      evidenceCollectionCount: 1,
      messageCount: 1,
    });
  });

  it("ignores blank actors and empty evidence cycles", () => {
    expect(
      diagnosisCollaborationParticipants({
        ownerSubject: " ",
        conversation: [{ role: "user", actor_subject: " ", content: "Hello." }],
        evidenceTimeline: [{ turn_count: 1, actor_subject: "operator-1" }],
        supplementalEvidence: [],
      }),
    ).toEqual([]);
  });

  it("labels roles for compact UI tags", () => {
    expect(diagnosisCollaborationRoleLabel("supplemental_evidence")).toBe(
      "supplemental",
    );
    expect(diagnosisCollaborationRoleLabel("confirmation")).toBe("confirmed");
  });

  it("maps participants to local directory identities without replacing the audit subject", () => {
    const users = [
      directoryUser({
        active: false,
        department: "Platform",
        department_path: "IT/Platform/SRE",
        department_paths: ["IT/Platform/SRE", "IT/Shared"],
        display_name: "Alice Chen",
        job_title: "SRE Lead",
        section: "SRE",
        subject: " user-1 ",
        username: "alice",
      }),
    ];
    const profile = diagnosisCollaborationParticipantProfile(
      {
        subject: "user-1",
        roles: ["message"],
        isSystem: false,
        messageCount: 1,
        evidenceCollectionCount: 0,
        supplementalEvidenceCount: 0,
        confirmedConclusion: false,
      },
      diagnosisCollaborationDirectoryIndex(users),
    );

    expect(profile).toEqual({
      displayName: "Alice Chen",
      subject: "user-1",
      detailTags: [
        "user-1",
        "IT/Platform/SRE",
        "IT/Shared",
        "Platform",
        "SRE",
        "SRE Lead",
      ],
      active: false,
      matchedDirectoryUser: true,
    });
  });

  it("maps arbitrary actor subjects to local directory identities", () => {
    const users = [
      directoryUser({
        department: "Platform",
        display_name: "Alice Chen",
        subject: "user-1",
      }),
    ];

    expect(
      diagnosisCollaborationActorProfile(
        " user-1 ",
        diagnosisCollaborationDirectoryIndex(users),
      ),
    ).toMatchObject({
      displayName: "Alice Chen",
      subject: "user-1",
      matchedDirectoryUser: true,
    });
  });

  it("collects room-scoped directory projections without duplicate subjects", () => {
    expect(
      diagnosisCollaborationDirectoryUsersFromRooms([
        {
          session_id: "room-1",
          chat_session_id: 1,
          diagnosis_task_id: 1,
          evidence_snapshot_id: 1,
          workflow_id: "workflow-1",
          run_id: "run-1",
          task_status: "running",
          room_status: "open",
          turn_count: 1,
          started_at: "2026-06-26T08:00:00Z",
          last_activity_at: "2026-06-26T08:01:00Z",
          closed_at: null,
          close_reason: "",
          participant_directory_users: [
            directoryUser({
              display_name: "Exact Alice",
              id: 10,
              subject: " user-1 ",
            }),
          ],
          created_at: "2026-06-26T08:00:00Z",
          updated_at: "2026-06-26T08:01:00Z",
        },
        {
          session_id: "room-1-list",
          chat_session_id: 2,
          diagnosis_task_id: 2,
          evidence_snapshot_id: 2,
          workflow_id: "workflow-2",
          run_id: "run-2",
          task_status: "running",
          room_status: "open",
          turn_count: 1,
          started_at: "2026-06-26T08:00:00Z",
          last_activity_at: "2026-06-26T08:01:00Z",
          closed_at: null,
          close_reason: "",
          participant_directory_users: [
            directoryUser({
              display_name: "List Alice",
              id: 11,
              subject: "user-1",
            }),
            directoryUser({
              display_name: "Bob",
              id: 12,
              subject: "user-2",
            }),
          ],
          created_at: "2026-06-26T08:00:00Z",
          updated_at: "2026-06-26T08:01:00Z",
        },
      ]).map((user) => [user.subject.trim(), user.display_name]),
    ).toEqual([
      ["user-1", "Exact Alice"],
      ["user-2", "Bob"],
    ]);
  });

  it("projects current authorization directory users for actor display", () => {
    expect(
      diagnosisCollaborationDirectoryUsersFromDirectoryUsers([
        directoryUser({
          active: false,
          display_name: "Inactive Alice",
          email: "alice-inactive@example.test",
          external_id: "external-inactive",
          id: 10,
          subject: " user-1 ",
        }),
        directoryUser({
          active: true,
          department: "Platform",
          department_path: "IT/Platform",
          department_paths: ["IT/Platform"],
          display_name: "Alice Chen",
          email: "alice@example.test",
          external_id: "external-active",
          id: 11,
          job_title: "SRE",
          section: "Operations",
          subject: "user-1",
          username: "alice",
        }),
        directoryUser({
          id: 12,
          subject: " ",
        }),
      ]),
    ).toEqual([
      {
        active: true,
        department: "Platform",
        department_path: "IT/Platform",
        department_paths: ["IT/Platform"],
        display_name: "Alice Chen",
        job_title: "SRE",
        section: "Operations",
        subject: "user-1",
        username: "alice",
      },
    ]);
  });

  it("classifies OpenClarion-owned subjects as system actors", () => {
    expect(
      diagnosisCollaborationSubjectIsSystem("openclarion:auto-diagnosis"),
    ).toBe(true);
    expect(
      diagnosisCollaborationSubjectIsSystem("openclarion:alertmanager-webhook:1"),
    ).toBe(true);
    expect(
      diagnosisCollaborationSubjectIsSystem("openclarion.notification-worker"),
    ).toBe(true);
    expect(diagnosisCollaborationSubjectIsSystem("operator-1")).toBe(false);
  });

  it("falls back to subject when a participant has no local directory projection", () => {
    const profile = diagnosisCollaborationParticipantProfile(
      {
        subject: "external-reviewer",
        roles: ["message"],
        isSystem: false,
        messageCount: 1,
        evidenceCollectionCount: 0,
        supplementalEvidenceCount: 0,
        confirmedConclusion: false,
      },
      diagnosisCollaborationDirectoryIndex([]),
    );

    expect(profile).toEqual({
      displayName: "external-reviewer",
      subject: "external-reviewer",
      detailTags: [],
      active: null,
      matchedDirectoryUser: false,
    });
  });

  it("summarizes active, inactive, unsynced, and system participant identity coverage", () => {
    const participants = [
      participant({ subject: "active-operator" }),
      participant({ subject: "inactive-operator" }),
      participant({ subject: "external-reviewer" }),
      participant({ isSystem: true, subject: "openclarion:auto-diagnosis" }),
    ];
    const directoryUsersBySubject = diagnosisCollaborationDirectoryIndex([
      directoryUser({
        active: true,
        display_name: "Active Operator",
        subject: "active-operator",
      }),
      directoryUser({
        active: false,
        display_name: "Inactive Operator",
        subject: "inactive-operator",
      }),
    ]);

    expect(
      diagnosisCollaborationIdentityCoverage(
        participants,
        directoryUsersBySubject,
      ),
    ).toEqual({
      detail:
        "Review local directory sync before relying on this room for multi-operator attribution.",
      humanParticipants: 3,
      inactiveParticipants: 1,
      status: "review",
      summary:
        "1/3 active directory matches, 1 not synced, 1 inactive profile, 1 system actor",
      syncedParticipants: 1,
      systemActors: 1,
      unsyncedParticipants: 1,
    });
  });

  it("marks identity coverage ready when every human participant has an active local directory profile", () => {
    const participants = [
      participant({ subject: "operator-1" }),
      participant({ isSystem: true, subject: "openclarion:auto-diagnosis" }),
    ];
    const directoryUsersBySubject = diagnosisCollaborationDirectoryIndex([
      directoryUser({ active: true, subject: "operator-1" }),
    ]);

    expect(
      diagnosisCollaborationIdentityCoverage(
        participants,
        directoryUsersBySubject,
      ),
    ).toMatchObject({
      humanParticipants: 1,
      status: "ready",
      summary: "1/1 active directory matches, 1 system actor",
    });
  });

  it("marks identity coverage empty when only system actors are present", () => {
    expect(
      diagnosisCollaborationIdentityCoverage(
        [participant({ isSystem: true, subject: "openclarion:auto-diagnosis" })],
        diagnosisCollaborationDirectoryIndex([]),
      ),
    ).toMatchObject({
      humanParticipants: 0,
      status: "empty",
      summary: "1 system actor, no human participants",
      systemActors: 1,
    });
  });

  it("normalizes REST participant summaries for the collaboration panel", () => {
    expect(
      diagnosisCollaborationParticipantsFromSummary([
        {
          subject: "openclarion:auto-diagnosis",
          roles: ["assistant"],
          is_system: false,
          message_count: 1,
          evidence_collection_count: 0,
          supplemental_evidence_count: 0,
          confirmed_conclusion: false,
        },
        {
          subject: " owner-1 ",
          roles: ["confirmation", "message", "owner"],
          is_system: false,
          message_count: 2,
          evidence_collection_count: -1,
          supplemental_evidence_count: 0,
          confirmed_conclusion: true,
        },
      ]),
    ).toEqual([
      {
        subject: "owner-1",
        roles: ["owner", "message", "confirmation"],
        isSystem: false,
        messageCount: 2,
        evidenceCollectionCount: 0,
        supplementalEvidenceCount: 0,
        confirmedConclusion: true,
      },
      {
        subject: "openclarion:auto-diagnosis",
        roles: ["assistant"],
        isSystem: true,
        messageCount: 1,
        evidenceCollectionCount: 0,
        supplementalEvidenceCount: 0,
        confirmedConclusion: false,
      },
    ]);
  });
});

function directoryUser(overrides: Partial<DirectoryUser> = {}): DirectoryUser {
  return {
    active: true,
    created_at: "2026-06-26T08:00:00Z",
    department: "",
    department_external_ids: [],
    department_path: "",
    department_paths: [],
    display_name: "",
    email: "",
    external_id: "user-1",
    id: 1,
    job_title: "",
    provider: "ops_iam",
    section: "",
    subject: "user-1",
    synced_at: "2026-06-26T08:00:00Z",
    updated_at: "2026-06-26T08:00:00Z",
    username: "",
    ...overrides,
  };
}

function participant(
  overrides: Partial<DiagnosisCollaborationParticipant> = {},
): DiagnosisCollaborationParticipant {
  return {
    confirmedConclusion: false,
    evidenceCollectionCount: 0,
    isSystem: false,
    messageCount: 1,
    roles: ["message"],
    subject: "operator-1",
    supplementalEvidenceCount: 0,
    ...overrides,
  };
}
