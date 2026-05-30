#!/usr/bin/env bash
set -euo pipefail

# Validate GitHub issue templates so intake forms stay aligned with
# repository triage policy.

python3 - <<'PY'
import os
import re
import sys

TEMPLATES = {
    ".github/ISSUE_TEMPLATE/bug_report.md": {
        "name": "Bug report",
        "about": "Report a reproducible defect",
        "title": "bug: ",
        "labels": "bug",
        "sections": [
            "What happened?",
            "What did you expect?",
            "How can we reproduce it?",
            "Environment",
            "Additional context",
        ],
    },
    ".github/ISSUE_TEMPLATE/feature_request.md": {
        "name": "Feature request",
        "about": "Propose one focused capability",
        "title": "feat: ",
        "labels": "enhancement",
        "sections": [
            "Problem",
            "Proposal",
            "Alternatives considered",
            "Acceptance criteria",
        ],
    },
}


def fail(message: str) -> None:
    print(f"[issue-template-check] {message}", file=sys.stderr)
    sys.exit(1)


def split_front_matter(path: str, body: str) -> tuple[dict[str, str], str]:
    lines = body.splitlines()
    if not lines or lines[0] != "---":
        fail(f"{path}: missing opening front matter marker")
    for index in range(1, len(lines)):
        if lines[index] == "---":
            front = parse_front_matter(path, lines[1:index])
            rest = "\n".join(lines[index + 1 :])
            return front, rest
    fail(f"{path}: missing closing front matter marker")


def parse_front_matter(path: str, lines: list[str]) -> dict[str, str]:
    out: dict[str, str] = {}
    for line_no, line in enumerate(lines, start=2):
        if not line.strip():
            continue
        if line.startswith(" ") or line.startswith("\t"):
            fail(f"{path}:{line_no}: front matter keys must not be indented")
        if ":" not in line:
            fail(f"{path}:{line_no}: front matter row must be key: value")
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if key in out:
            fail(f"{path}:{line_no}: duplicate front matter key {key!r}")
        if not key or not value:
            fail(f"{path}:{line_no}: front matter key and value must be non-empty")
        if value.startswith('"') and value.endswith('"'):
            value = value[1:-1]
        out[key] = value
    return out


def has_section_content(markdown: str, name: str) -> tuple[bool, bool]:
    heading_re = re.compile(rf"^##[ \t]+{re.escape(name)}[ \t]*$")
    next_h2_re = re.compile(r"^##[ \t]+\S")
    fence_re = re.compile(r"^[ \t]*(```|~~~)")

    in_section = False
    found = False
    in_fence = False

    for raw_line in markdown.splitlines():
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


problems: list[str] = []
for path, expected in TEMPLATES.items():
    if not os.path.isfile(path):
        problems.append(f"{path}: must be a regular file")
        continue
    if os.path.islink(path):
        problems.append(f"{path}: must not be a symlink")
        continue
    try:
        with open(path, encoding="utf-8") as template_file:
            raw = template_file.read()
    except OSError as exc:
        print(f"[issue-template-check] cannot read {path}: {exc}", file=sys.stderr)
        sys.exit(2)

    front, markdown = split_front_matter(path, raw)
    for key in ("name", "about", "title", "labels"):
        got = front.get(key)
        want = expected[key]
        if got != want:
            problems.append(f"{path}: front matter {key} = {got!r}, want {want!r}")

    for section in expected["sections"]:
        found, has_content = has_section_content(markdown, section)
        if not found:
            problems.append(f"{path}: missing ## {section}")
        elif not has_content:
            problems.append(f"{path}: empty ## {section}")

if problems:
    print("[issue-template-check] issue template contract violations:", file=sys.stderr)
    for problem in problems:
        print(f"  {problem}", file=sys.stderr)
    sys.exit(1)

print("[issue-template-check] OK")
PY
