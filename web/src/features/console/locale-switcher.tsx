"use client";

import { CheckOutlined, GlobalOutlined } from "@ant-design/icons";
import { App as AntdApp, Button, Dropdown, Tooltip } from "antd";
import type { MenuProps } from "antd";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";

import { isAppLocale, type AppLocale } from "@/i18n/config";
import { requestSameOriginJSON } from "@/lib/api/browser";

export function LocaleSwitcher() {
  const locale = useLocale();
  const router = useRouter();
  const t = useTranslations("Common");
  const { message } = AntdApp.useApp();
  const [pending, setPending] = useState(false);
  const selectedLocale: AppLocale = isAppLocale(locale) ? locale : "en";
  const items: MenuProps["items"] = [
    {
      icon: selectedLocale === "zh-CN" ? <CheckOutlined /> : undefined,
      key: "zh-CN",
      label: t("chinese"),
    },
    {
      icon: selectedLocale === "en" ? <CheckOutlined /> : undefined,
      key: "en",
      label: t("english"),
    },
  ];

  async function switchLocale(nextLocale: string) {
    if (
      pending ||
      !isAppLocale(nextLocale) ||
      nextLocale === selectedLocale
    ) {
      return;
    }
    setPending(true);
    try {
      const result = await requestSameOriginJSON<void>("/api/locale", {
        method: "PUT",
        body: { locale: nextLocale },
      });
      if (!result.ok) {
        message.error(t("switchLanguageFailed"));
        return;
      }
      document.documentElement.lang = nextLocale;
      router.refresh();
    } finally {
      setPending(false);
    }
  }

  return (
    <Tooltip title={t("switchLanguage")}>
      <Dropdown
        menu={{
          items,
          onClick: ({ key }) => void switchLocale(key),
          selectable: true,
          selectedKeys: [selectedLocale],
        }}
        placement="bottomRight"
        trigger={["click"]}
      >
        <Button
          aria-label={`${t("switchLanguage")}: ${t("language")}`}
          icon={<GlobalOutlined />}
          loading={pending}
          type="text"
        />
      </Dropdown>
    </Tooltip>
  );
}
