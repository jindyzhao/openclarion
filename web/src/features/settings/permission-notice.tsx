"use client";

import { Alert } from "antd";

import type { SettingsNotice } from "./query-state";

export function ReadOnlyModeAlert({ notice }: { notice: SettingsNotice }) {
  return (
    <Alert
      description={notice.message}
      message="Read-only mode"
      role="status"
      showIcon
      type="warning"
    />
  );
}
