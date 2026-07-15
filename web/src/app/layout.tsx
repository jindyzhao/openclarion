import type { Metadata } from "next";
import { getLocale, getMessages, getTranslations } from "next-intl/server";
import type { ReactNode } from "react";

import { ConsoleShell } from "@/features/reports/report-shell";

import { AntdStyleRegistry } from "./antd-style-registry";
import "./globals.css";
import { Providers } from "./providers";

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations("Metadata");
  return {
    title: "OpenClarion",
    description: t("description"),
  };
}

export default async function RootLayout({
  children,
}: {
  children: ReactNode;
}) {
  const locale = await getLocale();
  const messages = await getMessages();
  return (
    <html lang={locale}>
      <body>
        <AntdStyleRegistry>
          <Providers locale={locale} messages={messages}>
            <ConsoleShell>{children}</ConsoleShell>
          </Providers>
        </AntdStyleRegistry>
      </body>
    </html>
  );
}
