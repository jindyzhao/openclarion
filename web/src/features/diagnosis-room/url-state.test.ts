import { describe, expect, it } from "vitest";

import {
  boundedURLTextValue,
  diagnosisRoomWeComAuthErrorSearchParam,
  diagnosisRoomWeComAutoLoginSearchParam,
  diagnosisRoomWeComLaunchContextSearchParam,
  diagnosisRoomAnchorHref,
  diagnosisRoomLinkHref,
  diagnosisRoomURLWithSelectedRoom,
  diagnosisRoomWeComReturnTo,
  diagnosisRoomURLWithoutOneShotParams,
} from "./url-state";

describe("diagnosis room URL state", () => {
  it("normalizes bounded URL text values", () => {
    expect(boundedURLTextValue("  diagnosis-session-1  ", 128)).toBe(
      "diagnosis-session-1",
    );
    expect(boundedURLTextValue(undefined, 128)).toBeUndefined();
    expect(boundedURLTextValue("   ", 128)).toBeUndefined();
    expect(boundedURLTextValue("a".repeat(129), 128)).toBeUndefined();
    expect(boundedURLTextValue("session\n1", 128)).toBeUndefined();
  });

  it("builds diagnosis room links with stable query ordering", () => {
    expect(
      diagnosisRoomLinkHref({
        evidenceSnapshotID: 9001,
        intent: "review_conclusion",
        sessionID: "diagnosis-session-9001",
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&intent=review_conclusion&session_id=diagnosis-session-9001",
    );

    expect(
      diagnosisRoomLinkHref({
        evidenceSnapshotID: 9002,
        intent: "alert_review",
      }),
    ).toBe("/diagnosis-room?evidence_snapshot_id=9002&intent=alert_review");
  });

  it("builds diagnosis room links with executable evidence plans", () => {
    expect(
      diagnosisRoomLinkHref({
        evidencePlan: {
          alert_source_profile_id: 3,
          limit: 5,
          reason: "Collect the current sibling alerts.",
          tool: "active_alerts",
        },
        evidenceSnapshotID: 9001,
        intent: "confidence_review",
        sessionID: "diagnosis-session-9001",
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&intent=confidence_review&session_id=diagnosis-session-9001&evidence_reason=Collect+the+current+sibling+alerts.&evidence_tool=active_alerts&evidence_source_profile_id=3&evidence_limit=5",
    );
  });

  it("builds diagnosis room links with supplemental follow-up requests", () => {
    expect(
      diagnosisRoomLinkHref({
        evidenceSnapshotID: 9001,
        intent: "confidence_review",
        sessionID: "diagnosis-session-9001",
        supplementalFollowUp: {
          detail: "Attach the current DBA remediation status.",
          label: "DBA status",
          priority: "high",
        },
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&intent=confidence_review&session_id=diagnosis-session-9001&follow_up_detail=Attach+the+current+DBA+remediation+status.&follow_up_label=DBA+status&follow_up_priority=high",
    );
  });

  it("builds diagnosis room links with stable anchors", () => {
    expect(
      diagnosisRoomAnchorHref({
        anchorID: "diagnosis-notification-timeline",
        evidenceSnapshotID: 9001,
        sessionID: "diagnosis-session-9001",
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&session_id=diagnosis-session-9001#diagnosis-notification-timeline",
    );
    expect(
      diagnosisRoomAnchorHref({
        anchorID: "#diagnosis-notification-timeline",
        sessionID: "diagnosis-session-9001",
      }),
    ).toBe(
      "/diagnosis-room?session_id=diagnosis-session-9001#diagnosis-notification-timeline",
    );
    expect(
      diagnosisRoomAnchorHref({
        anchorID: " ",
        sessionID: "diagnosis-session-9001",
      }),
    ).toBe("/diagnosis-room?session_id=diagnosis-session-9001");
  });

  it("normalizes Enterprise WeChat message return paths to IAM session auth", () => {
    expect(
      diagnosisRoomWeComReturnTo({
        pathname: "/diagnosis-room",
        search: "",
      }),
    ).toBe("/diagnosis-room?auth_mode=session");

    expect(
      diagnosisRoomWeComReturnTo({
        pathname: "/diagnosis-room",
        search: "session_id=diagnosis-session-1&evidence_snapshot_id=7",
      }),
    ).toBe(
      "/diagnosis-room?session_id=diagnosis-session-1&evidence_snapshot_id=7&auth_mode=session",
    );

    expect(
      diagnosisRoomWeComReturnTo({
        pathname: "/diagnosis-room",
        search: "auth_mode=ldap&report_id=101",
      }),
    ).toBe("/diagnosis-room?auth_mode=session&report_id=101");

    expect(
      diagnosisRoomWeComReturnTo({
        pathname: "/diagnosis-room",
        search:
          "auth_mode=wecom&wecom_auth_error=wecom_callback_failed&wecom_auto_login=1&wecom_launch_context=workbench",
      }),
    ).toBe(
      "/diagnosis-room?auth_mode=session&wecom_launch_context=workbench",
    );

    expect(
      diagnosisRoomWeComReturnTo({
        pathname: "/diagnosis-room",
        search: "auth_mode=wecom&wecom_launch_context=browser",
      }),
    ).toBe("/diagnosis-room?auth_mode=session");
  });

  it("parses only known Enterprise WeChat auth errors", () => {
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=wecom_auth_failed"),
      ),
    ).toBe("wecom_auth_failed");
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=wecom_callback_failed"),
      ),
    ).toBe("wecom_callback_failed");
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=wecom_callback_missing"),
      ),
    ).toBe("wecom_callback_missing");
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=wecom_entry_unavailable"),
      ),
    ).toBe("wecom_entry_unavailable");
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=wecom_login_failed"),
      ),
    ).toBe("wecom_login_failed");
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=wecom_role_unauthorized"),
      ),
    ).toBe("wecom_role_unauthorized");
    expect(
      diagnosisRoomWeComAuthErrorSearchParam(
        new URLSearchParams("wecom_auth_error=provider-secret"),
      ),
    ).toBeUndefined();
  });

  it("parses and removes one-shot Enterprise WeChat auto login state", () => {
    expect(
      diagnosisRoomWeComAutoLoginSearchParam(
        new URLSearchParams("wecom_auto_login=1"),
      ),
    ).toBe(true);
    expect(
      diagnosisRoomWeComAutoLoginSearchParam(
        new URLSearchParams("wecom_auto_login=true"),
      ),
    ).toBe(false);
    expect(
      diagnosisRoomURLWithoutOneShotParams({
        pathname: "/diagnosis-room",
        search:
          "session_id=session-1&auth_mode=wecom&wecom_auto_login=1&wecom_launch_context=app_conversation&wecom_auth_error=wecom_callback_failed",
        authError: true,
        weComAutoLogin: true,
      }),
    ).toBe(
      "/diagnosis-room?session_id=session-1&auth_mode=session&wecom_launch_context=app_conversation",
    );
    expect(
      diagnosisRoomURLWithoutOneShotParams({
        pathname: "/diagnosis-room",
        search:
          "auth_mode=wecom&wecom_auto_login=1&wecom_launch_context=browser",
        weComAutoLogin: true,
      }),
    ).toBe("/diagnosis-room?auth_mode=session");
  });

  it("parses only known Enterprise WeChat launch contexts used by notification links", () => {
    expect(
      diagnosisRoomWeComLaunchContextSearchParam(
        new URLSearchParams("wecom_launch_context=wecom"),
      ),
    ).toBe("wecom");
    expect(
      diagnosisRoomWeComLaunchContextSearchParam(
        new URLSearchParams("wecom_launch_context=workbench"),
      ),
    ).toBe("workbench");
    expect(
      diagnosisRoomWeComLaunchContextSearchParam(
        new URLSearchParams("wecom_launch_context=app_conversation"),
      ),
    ).toBe("app_conversation");
    expect(
      diagnosisRoomWeComLaunchContextSearchParam(
        new URLSearchParams("wecom_launch_context=browser"),
      ),
    ).toBeUndefined();
  });

  it("keeps report context and one-shot params when selecting the same evidence snapshot", () => {
    expect(
      diagnosisRoomURLWithSelectedRoom({
        evidenceSnapshotID: 9001,
        keepReportContext: true,
        pathname: "/diagnosis-room",
        search:
          "evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&follow_up_label=DBA&follow_up_detail=Need+DBA+confirmation&follow_up_priority=high&evidence_tool=metric_query&evidence_reason=Need+CPU&evidence_query=up",
        sessionID: "diagnosis-session-9001",
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&follow_up_label=DBA&follow_up_detail=Need+DBA+confirmation&follow_up_priority=high&evidence_tool=metric_query&evidence_reason=Need+CPU&evidence_query=up&session_id=diagnosis-session-9001",
    );
  });

  it("removes report and one-shot params when selecting a different evidence snapshot", () => {
    expect(
      diagnosisRoomURLWithSelectedRoom({
        evidenceSnapshotID: 9002,
        keepReportContext: false,
        pathname: "/diagnosis-room",
        search:
          "evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&follow_up_label=DBA&follow_up_detail=Need+DBA+confirmation&follow_up_priority=high&evidence_tool=metric_query&evidence_reason=Need+CPU&evidence_query=up",
        sessionID: "diagnosis-session-9002",
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9002&session_id=diagnosis-session-9002",
    );
  });

  it("removes only selected one-shot params after use or clear", () => {
    const search =
      "evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&wecom_auth_error=wecom_callback_failed&follow_up_label=DBA&follow_up_detail=Need+DBA+confirmation&follow_up_priority=high&evidence_tool=metric_query&evidence_reason=Need+CPU&evidence_query=up";

    expect(
      diagnosisRoomURLWithoutOneShotParams({
        authError: true,
        pathname: "/diagnosis-room",
        search,
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&follow_up_label=DBA&follow_up_detail=Need+DBA+confirmation&follow_up_priority=high&evidence_tool=metric_query&evidence_reason=Need+CPU&evidence_query=up",
    );

    expect(
      diagnosisRoomURLWithoutOneShotParams({
        pathname: "/diagnosis-room",
        search,
        supplementalFollowUp: true,
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&wecom_auth_error=wecom_callback_failed&evidence_tool=metric_query&evidence_reason=Need+CPU&evidence_query=up",
    );

    expect(
      diagnosisRoomURLWithoutOneShotParams({
        evidencePlan: true,
        pathname: "/diagnosis-room",
        search,
      }),
    ).toBe(
      "/diagnosis-room?evidence_snapshot_id=9001&report_id=101&sub_report_id=501&intent=confidence_review&wecom_auth_error=wecom_callback_failed&follow_up_label=DBA&follow_up_detail=Need+DBA+confirmation&follow_up_priority=high",
    );
  });
});
