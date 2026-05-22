#!/usr/bin/env bash
# Validate relative markdown links in governed documentation.
#
# Scope:
#   - All *.md files under docs/, plus the top-level governance files:
#     README.md, CONTRIBUTING.md, GOVERNANCE.md, SECURITY.md,
#     CODE_OF_CONDUCT.md, DCO.md, MAINTAINERS.md, ADOPTERS.md, QA.md,
#     DEVELOPMENT_WORKFLOW.md.
#
# Rules:
#   - Extract `[text](target)` markdown links.
#   - Skip absolute URLs (http://, https://, mailto:, tel:, ftp://).
#   - Skip pure anchors (target starts with `#`).
#   - For relative targets:
#       * Strip any `#anchor` fragment.
#       * Resolve against the source file's directory.
#       * The resolved path must exist on disk.
#   - Anchors are not validated (would require a markdown AST parser).
#
# This is a string-level lint; fenced code blocks and inline code spans
# are stripped before extraction to reduce false positives.

set -euo pipefail

cd "$(dirname "$0")/.."

python3 - <<'PY'
import os
import re
import sys
from pathlib import Path

ROOT = Path(".").resolve()

GOVERNANCE_TOP = [
    "README.md", "CONTRIBUTING.md", "GOVERNANCE.md", "SECURITY.md",
    "CODE_OF_CONDUCT.md", "DCO.md", "MAINTAINERS.md", "ADOPTERS.md",
    "QA.md", "DEVELOPMENT_WORKFLOW.md",
]

files = []
docs = ROOT / "docs"
if docs.is_dir():
    for p in sorted(docs.rglob("*.md")):
        files.append(p)
for name in GOVERNANCE_TOP:
    p = ROOT / name
    if p.is_file():
        files.append(p)

if not files:
    print("[links-check] no markdown files; skipping.")
    sys.exit(0)

LINK_RE = re.compile(r"\[([^\]]+)\]\(([^)\s]+)\)")
SCHEME_SKIP = ("http://", "https://", "mailto:", "tel:", "ftp://")

broken = []
total_links = 0

for src in files:
    text = src.read_text(encoding="utf-8")
    # Strip fenced code blocks first, then inline code spans.
    text_stripped = re.sub(r"```.*?```", "", text, flags=re.DOTALL)
    text_stripped = re.sub(r"`[^`\n]*`", "", text_stripped)

    for m in LINK_RE.finditer(text_stripped):
        target = m.group(2)
        total_links += 1
        if target.startswith(SCHEME_SKIP):
            continue
        if target.startswith("#"):
            continue
        path_part = target.split("#", 1)[0]
        if not path_part:
            continue
        if path_part.startswith("/"):
            resolved = (ROOT / path_part.lstrip("/")).resolve(strict=False)
        else:
            resolved = (src.parent / path_part).resolve(strict=False)
        # Approximate line number for the original text.
        line_no = text_stripped.count("\n", 0, m.start()) + 1
        if not resolved.exists():
            try:
                rel_resolved = resolved.relative_to(ROOT)
            except ValueError:
                rel_resolved = resolved
            broken.append((str(src.relative_to(ROOT)), line_no, target, str(rel_resolved)))

if broken:
    print("[links-check] broken markdown links detected:", file=sys.stderr)
    for src, line, target, resolved in broken:
        print(f"  {src}:{line} -> {target}  (resolved: {resolved})", file=sys.stderr)
    print("", file=sys.stderr)
    print("Fix: update the link target or the document path.", file=sys.stderr)
    sys.exit(1)

print(f"[links-check] OK ({len(files)} files, {total_links} links scanned)")
PY
