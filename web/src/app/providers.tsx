"use client";

import "@ant-design/v5-patch-for-react-19";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { App as AntdApp, ConfigProvider, theme } from "antd";
import enUS from "antd/locale/en_US";
import zhCN from "antd/locale/zh_CN";
import { NextIntlClientProvider } from "next-intl";
import type { AbstractIntlMessages } from "next-intl";
import { ReactNode, useState } from "react";

import { appTimeZone } from "@/i18n/config";

export function Providers({
  children,
  locale,
  messages,
}: {
  children: ReactNode;
  locale: string;
  messages: AbstractIntlMessages;
}) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          mutations: {
            retry: false
          },
          queries: {
            refetchOnWindowFocus: false,
            retry: 1,
            staleTime: 60_000
          }
        }
      })
  );

  return (
    <NextIntlClientProvider
      locale={locale}
      messages={messages}
      timeZone={appTimeZone}
    >
      <QueryClientProvider client={queryClient}>
        <ConfigProvider
          locale={locale === "zh-CN" ? zhCN : enUS}
          theme={{
            algorithm: theme.defaultAlgorithm,
            token: {
              borderRadius: 8,
              colorBgContainer: "#ffffff",
              colorBgLayout: "#f7f8fa",
              colorBorder: "#d7dde3",
              colorError: "#b42318",
              colorInfo: "#2563eb",
              colorLink: "#1d4ed8",
              colorPrimary: "#0f766e",
              colorSuccess: "#087443",
              colorText: "#17202a",
              colorTextSecondary: "#5d6b79",
              colorWarning: "#a15c07",
              fontFamily:
                'Inter, "PingFang SC", "Microsoft YaHei", ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
              lineHeight: 1.55,
            },
          }}
        >
          <AntdApp>{children}</AntdApp>
        </ConfigProvider>
      </QueryClientProvider>
    </NextIntlClientProvider>
  );
}
