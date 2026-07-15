"use client";

import { CheckOutlined, GlobalOutlined } from "@ant-design/icons";
import { Button, Dropdown, Tooltip } from "antd";
import type { MenuProps } from "antd";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useTransition } from "react";

import {
  appLocaleCookieName,
  isAppLocale,
  type AppLocale,
} from "@/i18n/config";

export function LocaleSwitcher() {
  const locale = useLocale();
  const router = useRouter();
  const t = useTranslations("Common");
  const [pending, startTransition] = useTransition();
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

  function switchLocale(nextLocale: string) {
    if (!isAppLocale(nextLocale) || nextLocale === selectedLocale) {
      return;
    }
    const secure = globalThis.location.protocol === "https:" ? "; secure" : "";
    document.cookie = `${appLocaleCookieName}=${nextLocale}; max-age=31536000; path=/; samesite=lax${secure}`;
    document.documentElement.lang = nextLocale;
    startTransition(() => router.refresh());
  }

  return (
    <Tooltip title={t("switchLanguage")}>
      <Dropdown
        menu={{
          items,
          onClick: ({ key }) => switchLocale(key),
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
