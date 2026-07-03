import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "playwright-report/**",
    "playwright-report-live/**",
    "test-results/**",
    "test-results-live/**",
    "next-env.d.ts",
    "tsconfig.tsbuildinfo",
    "src/lib/api/openapi.ts"
  ])
]);

export default eslintConfig;
