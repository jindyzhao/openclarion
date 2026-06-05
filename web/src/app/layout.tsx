import type { Metadata } from "next";
import type { ReactNode } from "react";

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
          <Providers>{children}</Providers>
        </AntdStyleRegistry>
      </body>
    </html>
  );
}
