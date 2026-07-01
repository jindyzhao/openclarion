"use client";

import { Collapse, Segmented, Space, Tag, Typography } from "antd";
import { useMemo, useState } from "react";

import type {
  DiagnosisAuthMode,
  DiagnosisAuthModeOption,
} from "./auth-readiness";

export function DiagnosisAuthModeSelector({
  disabled = false,
  onChange,
  options,
  value,
}: {
  disabled?: boolean;
  onChange?: (value: DiagnosisAuthMode) => void;
  options: DiagnosisAuthModeOption[];
  value?: DiagnosisAuthMode;
}) {
  const current = value ?? "session";
  const sessionOption =
    options.find((option) => option.value === "session") ?? null;
  const fallbackOptions = useMemo(
    () => options.filter((option) => option.value !== "session"),
    [options],
  );
  const selectedFallbackOption =
    fallbackOptions.find((option) => option.value === current) ?? null;
  const [fallbackOpen, setFallbackOpen] = useState(false);
  const fallbackActiveKey =
    fallbackOpen || current !== "session" ? ["fallback-auth"] : [];
  const onFallbackCollapseChange = (keys: string | string[]) => {
    setFallbackOpen(
      Array.isArray(keys)
        ? keys.includes("fallback-auth")
        : keys === "fallback-auth",
    );
  };

  return (
    <Space direction="vertical" size={8} style={{ width: "100%" }}>
      <Segmented<DiagnosisAuthMode>
        block
        disabled={disabled}
        onChange={(next) => onChange?.(next)}
        options={[
          {
            disabled: sessionOption?.disabled,
            label: sessionOption?.label ?? "IAM session",
            value: "session",
          },
        ]}
        value={current === "session" ? "session" : undefined}
      />
      <Collapse
        activeKey={fallbackActiveKey}
        ghost
        items={[
          {
            children: (
              <Space direction="vertical" size={8} style={{ width: "100%" }}>
                <Typography.Text type="secondary">
                  Use these paths only for staged rollout, debug access, or a
                  deployment where IAM OIDC is not active.
                </Typography.Text>
                <Segmented<DiagnosisAuthMode>
                  block
                  disabled={disabled}
                  onChange={(next) => onChange?.(next)}
                  options={fallbackOptions}
                  value={current === "session" ? undefined : current}
                />
              </Space>
            ),
            extra:
              selectedFallbackOption === null ? (
                <Tag>optional</Tag>
              ) : (
                <Tag
                  color={diagnosisAuthModeTagColor(
                    selectedFallbackOption.value,
                  )}
                >
                  {selectedFallbackOption.label}
                </Tag>
              ),
            forceRender: true,
            key: "fallback-auth",
            label: "Fallback auth methods",
          },
        ]}
        onChange={onFallbackCollapseChange}
        size="small"
      />
    </Space>
  );
}

function diagnosisAuthModeTagColor(mode: DiagnosisAuthMode): string {
  switch (mode) {
    case "ldap":
      return "blue";
    case "bearer":
      return "default";
    case "session":
      return "gold";
    case "wecom":
      return "green";
  }
}
