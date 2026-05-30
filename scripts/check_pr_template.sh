#!/usr/bin/env bash
set -euo pipefail

# Validate that the repository pull request template matches the PR body gates.

python3 - <<'PY'
import os
import re
import sys

TEMPLATE = ".github/pull_request_template.md"
REQUIRED_SECTIONS = [
    "Summary",
    "Risk",
    "Rollback",
    "Local verification",
    "DCO",
]


def fail(message: str) -> None:
    print(f"[pr-template-check] {message}", file=sys.stderr)
    sys.exit(1)


def has_section_content(body: str, name: str) -> tuple[bool, bool]:
    heading_re = re.compile(rf"^##[ \t]+{re.escape(name)}[ \t]*$")
    next_h2_re = re.compile(r"^##[ \t]+\S")
    fence_re = re.compile(r"^[ \t]*(```|~~~)")

    in_section = False
    found = False
    in_fence = False

    for raw_line in body.splitlines():
        line = raw_line.rstrip("\r")
        stripped = line.strip()

        if fence_re.match(line):
            in_fence = not in_fence
            continue

        if not in_fence:
            if heading_re.match(line):
                found = True
                in_section = True
                continue
            if in_section and next_h2_re.match(line):
                break

        if in_section and stripped and not re.fullmatch(r"<!--.*-->", stripped):
            return True, True

    return found, False


if not os.path.isfile(TEMPLATE):
    fail(f"{TEMPLATE} must be a regular file")
if os.path.islink(TEMPLATE):
    fail(f"{TEMPLATE} must not be a symlink")

try:
    with open(TEMPLATE, encoding="utf-8") as template_file:
        body = template_file.read()
except OSError as exc:
    print(f"[pr-template-check] cannot read {TEMPLATE}: {exc}", file=sys.stderr)
    sys.exit(2)

missing: list[str] = []
empty: list[str] = []
for section in REQUIRED_SECTIONS:
    found, has_content = has_section_content(body, section)
    if not found:
        missing.append(section)
    elif not has_content:
        empty.append(section)

if missing or empty:
    print("[pr-template-check] pull request template must include non-empty sections:", file=sys.stderr)
    for section in missing:
        print(f"  missing ## {section}", file=sys.stderr)
    for section in empty:
        print(f"  empty ## {section}", file=sys.stderr)
    sys.exit(1)

print("[pr-template-check] OK")
PY
