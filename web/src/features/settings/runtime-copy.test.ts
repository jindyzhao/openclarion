import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCNMessages from "../../../messages/zh-CN.json";
import { localizeAlertSourceText } from "./alert-sources/alert-source-settings-view";
import { localizeDiagnosisToolText } from "./diagnosis-tool-templates/diagnosis-tool-template-settings-view";
import { localizeDirectoryText } from "./directory-rbac/directory-rbac-settings-view";
import { localizeNotificationChannelText } from "./notification-channels/notification-channel-settings-view";
import { localizeWorkflowPolicyText } from "./report-workflow-policies/report-workflow-policy-settings-view";
import { localizeWorkflowScheduleText } from "./report-workflow-schedules/report-workflow-schedule-settings-view";

const alertSourceT = createTranslator({
  locale: "zh-CN",
  messages: zhCNMessages,
  namespace: "AlertSourceSettings",
});
const diagnosisToolT = createTranslator({
  locale: "zh-CN",
  messages: zhCNMessages,
  namespace: "DiagnosisToolSettings",
});
const directoryT = createTranslator({
  locale: "zh-CN",
  messages: zhCNMessages,
  namespace: "DirectorySettings",
});
const notificationChannelT = createTranslator({
  locale: "zh-CN",
  messages: zhCNMessages,
  namespace: "NotificationChannelSettings",
});
const workflowPolicyT = createTranslator({
  locale: "zh-CN",
  messages: zhCNMessages,
  namespace: "WorkflowPolicySettings",
});
const workflowScheduleT = createTranslator({
  locale: "zh-CN",
  messages: zhCNMessages,
  namespace: "WorkflowScheduleSettings",
});

describe("settings runtime copy", () => {
  it("localizes alert-source workflow counts and controlled source labels", () => {
    const t = alertSourceT;

    expect(
      localizeAlertSourceText(
        "2 enabled active_alerts template(s) are bound to this source.",
        t,
      ),
    ).toBe("此告警源已绑定 2 个已启用的 active_alerts 模板。");
    expect(
      localizeAlertSourceText(
        "Alertmanager source is saved and enabled.",
        t,
      ),
    ).toBe("Alertmanager 告警源已保存并启用。");
    expect(
      localizeAlertSourceText(
        "3 automatic diagnosis workflow(s) exist for this source but are disabled.",
        t,
      ),
    ).toBe("此告警源存在 3 个自动诊断工作流，但均未启用。");
  });

  it("localizes diagnosis-tool guidance while preserving source names", () => {
    const t = diagnosisToolT;

    expect(
      localizeDiagnosisToolText(
        "Range metric needs an enabled Prometheus-compatible source before it can collect evidence.",
        t,
      ),
    ).toBe("范围指标需要已启用的 Prometheus 兼容告警源才能采集证据。");
    expect(
      t("catalog.recommendationDetail", {
        label: localizeDiagnosisToolText("Kubernetes pod CPU range", t),
        source: "Source grouped by Legacy",
      }),
    ).toContain("Source grouped by Legacy");
  });

  it("localizes directory scope metadata without changing scope keys", () => {
    const t = directoryT;

    expect(localizeDirectoryText("Department / team-prod", t)).toBe(
      "部门 / team-prod",
    );
    expect(localizeDirectoryText("#7 Payments (disabled)", t)).toBe(
      "#7 Payments（已停用）",
    );
  });

  it("localizes notification inventory counts", () => {
    const t = notificationChannelT;

    expect(
      localizeNotificationChannelText(
        "2 AI diagnosis delivery channels can be used now; 1 candidate channel still need setup or proof review.",
        t,
      ),
    ).toBe(
      "当前可使用 2 个 AI 诊断交付渠道；仍有 1 个候选渠道需要完成配置或证明检查。",
    );
  });

  it("localizes workflow guidance and preserves configured names", () => {
    const t = workflowPolicyT;

    expect(
      localizeWorkflowPolicyText(
        "Saves Ops for DB for #1 Source A grouped by #2 Group B.",
        t,
      ),
    ).toBe("将 Ops for DB 保存为使用 #1 Source A 并按 #2 Group B 分组的策略。");
    expect(
      localizeWorkflowPolicyText(
        "Alert storm reports use #1 Source A and #2 Group B.",
        t,
      ),
    ).toBe("告警风暴报告使用 #1 Source A 和 #2 Group B。");
    expect(
      localizeWorkflowPolicyText(
        "Current user is not authorized to read alert source #3, test notification channel #4.",
        t,
      ),
    ).toBe("当前用户无权执行以下操作：读取告警源 #3、测试通知渠道 #4。");
    expect(localizeWorkflowPolicyText("#4 missing report", t)).toBe(
      "#4 缺少报告范围",
    );
    expect(
      localizeWorkflowPolicyText(
        "Ops channel: Notification channel disabled.",
        t,
      ),
    ).toBe("Ops channel：通知渠道已停用。");
  });

  it("localizes schedule fallback policy identifiers", () => {
    expect(
      localizeWorkflowScheduleText(
        "Policy #9",
        workflowScheduleT,
      ),
    ).toBe("策略 #9");
  });
});
