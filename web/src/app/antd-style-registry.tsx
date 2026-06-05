"use client";

import { createCache, extractStyle, StyleProvider } from "@ant-design/cssinjs";
import { useServerInsertedHTML } from "next/navigation";
import { ReactNode, useState } from "react";

export function AntdStyleRegistry({ children }: { children: ReactNode }) {
  const [cache] = useState(() => createCache());

  useServerInsertedHTML(() => {
    const styleText = extractStyle(cache, {
      plain: true,
      once: true
    });
    if (styleText.includes('.data-ant-cssinjs-cache-path{content:"";}')) {
      return null;
    }
    return (
      <style
        data-rc-order="prepend"
        data-rc-priority="-1000"
        dangerouslySetInnerHTML={{ __html: styleText }}
        id="antd-cssinjs"
      />
    );
  });

  return <StyleProvider cache={cache}>{children}</StyleProvider>;
}
