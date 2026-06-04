#!/usr/bin/env bash
set -euo pipefail

# Validate that pull request descriptions include explicit risk and rollback
# sections. Local usage:
#   PR_BODY="$(gh pr view --json body --jq .body)" make pr-description-check

python3 - <<'PY'
import json
import os
import re
import stat
import sys


def reject_duplicate_object_keys(pairs):
    obj = {}
    seen = set()
    for key, value in pairs:
        if key in seen:
            raise ValueError(f"duplicate JSON key: {key}")
        seen.add(key)
        obj[key] = value
    return obj


def load_event(path: str):
    try:
        mode = os.lstat(path).st_mode
    except OSError as exc:
        print(f"[pr-description-check] cannot read GITHUB_EVENT_PATH: {exc}", file=sys.stderr)
        sys.exit(2)
    if stat.S_ISLNK(mode):
        print("[pr-description-check] GITHUB_EVENT_PATH must be a regular file, not a symlink.", file=sys.stderr)
        sys.exit(2)
    if not stat.S_ISREG(mode):
        print("[pr-description-check] GITHUB_EVENT_PATH must be a regular file.", file=sys.stderr)
        sys.exit(2)
    try:
        with open(path, encoding="utf-8") as event_file:
            return json.load(event_file, object_pairs_hook=reject_duplicate_object_keys)
    except OSError as exc:
        print(f"[pr-description-check] cannot read GITHUB_EVENT_PATH: {exc}", file=sys.stderr)
        sys.exit(2)
    except json.JSONDecodeError as exc:
        print(f"[pr-description-check] invalid GITHUB_EVENT_PATH JSON: {exc}", file=sys.stderr)
        sys.exit(2)
    except ValueError as exc:
        print(f"[pr-description-check] invalid GITHUB_EVENT_PATH JSON: {exc}", file=sys.stderr)
        sys.exit(2)


def load_body() -> str:
    body = os.environ.get("PR_BODY")
    if body is not None:
        return body

    event_path = os.environ.get("GITHUB_EVENT_PATH")
    if event_path:
        event = load_event(event_path)
        if not isinstance(event, dict):
            print("[pr-description-check] GITHUB_EVENT_PATH JSON root must be an object.", file=sys.stderr)
            sys.exit(2)
        pull_request = event.get("pull_request") or {}
        if not isinstance(pull_request, dict):
            print("[pr-description-check] GITHUB_EVENT_PATH pull_request must be an object.", file=sys.stderr)
            sys.exit(2)
        body = pull_request.get("body") or ""
        if not isinstance(body, str):
            print("[pr-description-check] GITHUB_EVENT_PATH pull_request.body must be a string or null.", file=sys.stderr)
            sys.exit(2)
        return body

    print("[pr-description-check] PR_BODY or GITHUB_EVENT_PATH is required.", file=sys.stderr)
    print("[pr-description-check] Usage: PR_BODY='<body with ## Risk and ## Rollback>' make pr-description-check", file=sys.stderr)
    sys.exit(2)


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
            if in_section and not in_fence and stripped not in {"```", "~~~"}:
                return True, True
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


body = load_body()
if not body.strip():
    print("[pr-description-check] PR description is empty.", file=sys.stderr)
    print("Expected non-empty ## Risk and ## Rollback sections.", file=sys.stderr)
    sys.exit(1)

missing: list[str] = []
empty: list[str] = []
for section in ("Risk", "Rollback"):
    found, has_content = has_section_content(body, section)
    if not found:
        missing.append(section)
    elif not has_content:
        empty.append(section)

if missing or empty:
    print("[pr-description-check] PR description must include non-empty sections:", file=sys.stderr)
    for section in missing:
        print(f"  missing ## {section}", file=sys.stderr)
    for section in empty:
        print(f"  empty ## {section}", file=sys.stderr)
    print("", file=sys.stderr)
    print("Expected template:", file=sys.stderr)
    print("  ## Risk", file=sys.stderr)
    print("  <risk summary, or 'None' with rationale>", file=sys.stderr)
    print("", file=sys.stderr)
    print("  ## Rollback", file=sys.stderr)
    print("  <rollback plan, or 'Revert this PR'>", file=sys.stderr)
    sys.exit(1)

print("[pr-description-check] OK")
PY
