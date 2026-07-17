import { readFileSync, readdirSync } from "node:fs";
import { extname, join, relative } from "node:path";

import ts from "typescript";
import { describe, expect, it } from "vitest";

const sourceRoot = join(process.cwd(), "src");
const productionUIRoots = [join(sourceRoot, "app"), join(sourceRoot, "features")];
const localeBranchAllowlist = new Set([
  "app/api/locale/route.ts",
  "app/providers.tsx",
  "features/console/locale-switcher.tsx",
]);
const userFacingJSXAttributes = new Set([
  "aria-label",
  "cancelText",
  "description",
  "label",
  "message",
  "okText",
  "placeholder",
  "title",
]);
const productNameAllowlist = new Set(["Alertmanager", "Prometheus"]);
const technicalPlaceholderAllowlist = new Set([
  "2026-06-05T08:00:00Z",
  "2026-06-05T09:00:00Z",
  "2026-06-26T08:00:00Z",
  "alertname\nservice",
  "dep-1, dep-2",
  "env=prod\nowner=platform",
  "openclarion-report-policy-1-daily",
  "prometheus\nalertmanager",
  "rate(container_cpu_usage_seconds_total[5m])",
  "secret/example/ops-webhook",
  "team=ops",
]);
const supportedLocaleLiterals = new Set(["en", "zh", "zh-CN"]);
const productionSources = productionSourceFiles().map((file) => {
  const source = readFileSync(file, "utf8");
  return {
    file,
    relativePath: relativeSourcePath(file),
    source,
    tree: ts.createSourceFile(
      file,
      source,
      ts.ScriptTarget.Latest,
      true,
      extname(file) === ".tsx" ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
    ),
  };
});

describe("strict UI catalog usage", () => {
  it("keeps locale-specific copy out of production UI source", () => {
    const violations: string[] = [];
    for (const { relativePath, source } of productionSources) {
      if (/\p{Script=Han}/u.test(source)) {
        violations.push(`${relativePath} contains Han copy`);
      }
    }
    expect(violations).toEqual([]);
  });

  it("keeps language branching at locale infrastructure boundaries", () => {
    const violations: string[] = [];
    for (const { relativePath, tree } of productionSources) {
      if (
        !localeBranchAllowlist.has(relativePath) &&
        sourceContainsLocaleBranch(tree)
      ) {
        violations.push(relativePath);
      }
    }
    expect(violations).toEqual([]);
  });

  it("does not render hard-coded prose as JSX text", () => {
    const violations: string[] = [];
    for (const { file, tree } of productionSources.filter(
      (candidate) => extname(candidate.file) === ".tsx",
    )) {
      const inspect = (node: ts.Node) => {
        if (ts.isJsxText(node)) {
          const text = node.getText(tree).replace(/\s+/g, " ").trim();
          if (
            /[A-Za-z]{2,}/.test(text) &&
            !new Set(["OC", "OpenClarion"]).has(text)
          ) {
            const position = tree.getLineAndCharacterOfPosition(node.getStart(tree));
            violations.push(
              `${relativeSourcePath(file)}:${position.line + 1} renders ${JSON.stringify(text)}`,
            );
          }
        }
        ts.forEachChild(node, inspect);
      };
      inspect(tree);
    }
    expect(violations).toEqual([]);
  });

  it("does not render hard-coded prose in user-facing JSX attributes", () => {
    const violations: string[] = [];
    for (const { file, tree } of productionSources.filter(
      (candidate) => extname(candidate.file) === ".tsx",
    )) {
      const inspect = (node: ts.Node) => {
        if (
          ts.isJsxAttribute(node) &&
          userFacingJSXAttributes.has(node.name.getText(tree)) &&
          node.initializer !== undefined
        ) {
          const literal = jsxAttributeStringLiteral(node.initializer);
          const text = literal?.text.trim() ?? "";
          const attributeName = node.name.getText(tree);
          if (
            /[A-Za-z]{2,}/.test(text) &&
            !productNameAllowlist.has(text) &&
            !(
              attributeName === "placeholder" &&
              technicalPlaceholderAllowlist.has(text)
            )
          ) {
            const position = tree.getLineAndCharacterOfPosition(
              node.getStart(tree),
            );
            violations.push(
              `${relativeSourcePath(file)}:${position.line + 1} renders ${JSON.stringify(text)}`,
            );
          }
        }
        ts.forEachChild(node, inspect);
      };
      inspect(tree);
    }
    expect(violations).toEqual([]);
  });
});

function productionSourceFiles(): string[] {
  return productionUIRoots.flatMap(sourceFiles);
}

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return sourceFiles(path);
    }
    if (
      !entry.isFile() ||
      !/\.tsx?$/.test(entry.name) ||
      /\.(?:test|spec)\.tsx?$/.test(entry.name)
    ) {
      return [];
    }
    return [path];
  });
}

function relativeSourcePath(file: string): string {
  return relative(sourceRoot, file);
}

function sourceContainsLocaleBranch(tree: ts.SourceFile): boolean {
  let found = false;
  const inspect = (node: ts.Node) => {
    if (found) {
      return;
    }
    if (
      ts.isBinaryExpression(node) &&
      [
        ts.SyntaxKind.EqualsEqualsEqualsToken,
        ts.SyntaxKind.ExclamationEqualsEqualsToken,
        ts.SyntaxKind.EqualsEqualsToken,
        ts.SyntaxKind.ExclamationEqualsToken,
      ].includes(node.operatorToken.kind) &&
      ((isLocaleExpression(node.left, tree) && isLocaleLiteral(node.right)) ||
        (isLocaleExpression(node.right, tree) && isLocaleLiteral(node.left)))
    ) {
      found = true;
      return;
    }
    if (
      ts.isSwitchStatement(node) &&
      isLocaleExpression(node.expression, tree) &&
      node.caseBlock.clauses.some(
        (clause) => ts.isCaseClause(clause) && isLocaleLiteral(clause.expression),
      )
    ) {
      found = true;
      return;
    }
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      ["includes", "startsWith"].includes(node.expression.name.text) &&
      isLocaleExpression(node.expression.expression, tree) &&
      node.arguments.some(isLocaleLiteral)
    ) {
      found = true;
      return;
    }
    ts.forEachChild(node, inspect);
  };
  inspect(tree);
  return found;
}

function isLocaleExpression(node: ts.Node, tree: ts.SourceFile): boolean {
  return /locale/i.test(node.getText(tree));
}

function isLocaleLiteral(node: ts.Node): boolean {
  return ts.isStringLiteral(node) && supportedLocaleLiterals.has(node.text);
}

function jsxAttributeStringLiteral(
  initializer: ts.JsxAttributeValue,
): ts.StringLiteral | ts.NoSubstitutionTemplateLiteral | undefined {
  if (
    ts.isStringLiteral(initializer) ||
    ts.isNoSubstitutionTemplateLiteral(initializer)
  ) {
    return initializer;
  }
  const expression = ts.isJsxExpression(initializer)
    ? initializer.expression
    : undefined;
  return expression !== undefined &&
    (ts.isStringLiteral(expression) ||
      ts.isNoSubstitutionTemplateLiteral(expression))
    ? expression
    : undefined;
}
