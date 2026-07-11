import type { Metadata } from "next";
import type { ReactNode } from "react";

import { ConsoleShell } from "@/features/reports/report-shell";

import { AntdStyleRegistry } from "./antd-style-registry";
import "./globals.css";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: "OpenClarion",
  description: "OpenClarion report operations console"
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <AntdStyleRegistry>
          <Providers>
            <ConsoleShell>{children}</ConsoleShell>
          </Providers>
        </AntdStyleRegistry>
      </body>
    </html>
  );
}
