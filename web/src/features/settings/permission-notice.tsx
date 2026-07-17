"use client";

import { Alert } from "antd";
import { useTranslations } from "next-intl";

import type { SettingsNotice } from "./query-state";

export function ReadOnlyModeAlert({ notice }: { notice: SettingsNotice }) {
  const t = useTranslations("Common");
  return (
    <Alert
      description={notice.message}
      message={t("readOnlyMode")}
      role="status"
      showIcon
      type="warning"
    />
  );
}
