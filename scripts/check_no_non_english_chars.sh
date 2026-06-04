#!/usr/bin/env bash
set -euo pipefail

top_level_docs=(
  README.md
  DEVELOPMENT_WORKFLOW.md
  CONTRIBUTING.md
  GOVERNANCE.md
  SECURITY.md
  CODE_OF_CONDUCT.md
  DCO.md
  MAINTAINERS.md
)
governed_paths=("${top_level_docs[@]}" docs)

failed=0
for path in "${top_level_docs[@]}"; do
  if [[ -L "$path" || ( -e "$path" && ! -f "$path" ) ]]; then
    echo "[docs-hygiene] governed documentation file must be a regular file: $path" >&2
    failed=1
  elif [[ ! -e "$path" ]]; then
    echo "[docs-hygiene] governed documentation file is missing: $path" >&2
    failed=1
  fi
done

if [[ -L docs || ( -e docs && ! -d docs ) ]]; then
  echo "[docs-hygiene] governed documentation directory must be a real directory: docs" >&2
  failed=1
elif [[ ! -e docs ]]; then
  echo "[docs-hygiene] governed documentation directory is missing: docs" >&2
  failed=1
else
  mapfile -t indirect_docs < <(find docs \( -type l -o \( ! -type f ! -type d \) \) -print 2>/dev/null | sort)
  if [[ ${#indirect_docs[@]} -gt 0 ]]; then
    echo "[docs-hygiene] governed documentation paths must be regular files or directories:" >&2
    printf '%s\n' "${indirect_docs[@]}" >&2
    failed=1
  fi
fi

if [[ $failed -ne 0 ]]; then
  exit 1
fi

python3 - "${governed_paths[@]}" <<'PY'
import re
import sys
from pathlib import Path

pattern = re.compile(r"[\u2e80-\u9fff\uf900-\ufaff]")
matches = []

def scan_file(path):
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"[docs-hygiene] failed to read {path}: {exc}", file=sys.stderr)
        sys.exit(2)
    except UnicodeDecodeError as exc:
        print(f"[docs-hygiene] {path} must be UTF-8 text: {exc}", file=sys.stderr)
        sys.exit(2)
    for line_no, line in enumerate(text.splitlines(), start=1):
        if pattern.search(line):
            matches.append(f"{path}:{line_no}:{line}")

for raw in sys.argv[1:]:
    root = Path(raw)
    if root.is_dir():
        for path in sorted(root.rglob("*")):
            if path.is_file():
                scan_file(path)
    elif root.is_file():
        scan_file(root)

if matches:
    print("\n".join(matches))
    print("Non-English CJK characters found in governed documentation.", file=sys.stderr)
    sys.exit(1)
PY
