"use client";

import { Space, Tag, Typography } from "antd";
import { useTranslations } from "next-intl";

import {
  directorySubjectIsSystem,
  directorySubjectProfile,
} from "./format";
import type { DirectoryUser } from "./types";

export function DirectorySubjectTags({
  directoryUsersBySubject,
  label,
  subject,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DirectoryUser>;
  label?: string;
  subject?: string;
}) {
  const t = useTranslations("DirectorySettings");
  const normalizedSubject = subject?.trim() ?? "";
  if (normalizedSubject === "") {
    return null;
  }
  const profile = directorySubjectProfile(
    normalizedSubject,
    directoryUsersBySubject,
  );
  const isSystem = directorySubjectIsSystem(normalizedSubject);
  return (
    <Space size={[6, 6]} wrap>
      {label ? <Typography.Text type="secondary">{label}</Typography.Text> : null}
      <Tag color={profile.matchedDirectoryUser ? "processing" : "default"}>
        {profile.displayName}
      </Tag>
      {profile.displayName !== profile.subject ? (
        <Tag>{profile.subject}</Tag>
      ) : null}
      {profile.detailTags.slice(0, 2).map((tag) => (
        <Tag key={tag}>{tag}</Tag>
      ))}
      {profile.active === false ? <Tag color="warning">{t("inactive")}</Tag> : null}
      {!isSystem && !profile.matchedDirectoryUser ? (
        <Tag color="default">{t("notSynced")}</Tag>
      ) : null}
      {isSystem ? <Tag color="default">{t("system")}</Tag> : null}
    </Space>
  );
}
