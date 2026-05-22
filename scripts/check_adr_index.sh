#!/usr/bin/env bash
# Validate that docs/adr/README.md is consistent with files on disk.
#
# Checks:
#   1. Every ADR file ADR-NNNN-*.md is referenced by README.md.
#   2. Every README.md row points to a file that exists.
#   3. Every README.md row's link basename matches the displayed ID.
#   4. ADR ids in README.md are unique and monotonic (no gaps).
#   5. Every ADR file has Status: <one of accepted statuses>.
#
# This is a string-level lint; it does not parse markdown ASTs.

set -euo pipefail

cd "$(dirname "$0")/.."

readme="docs/adr/README.md"
adr_dir="docs/adr"

if [[ ! -f "$readme" ]]; then
  echo "[adr-check] $readme missing." >&2
  exit 1
fi

failed=0

# Files on disk: ADR-NNNN-*.md sorted.
mapfile -t disk_files < <(find "$adr_dir" -maxdepth 1 -type f \
  -name 'ADR-*.md' | sort)

# IDs referenced in README rows of the form `[ADR-NNNN](ADR-NNNN-*.md)`.
mapfile -t readme_ids < <(grep -oE '\[ADR-[0-9]{4}\]\(ADR-[0-9]{4}-[a-z0-9-]+\.md\)' "$readme" \
  | sed -E 's/^\[(ADR-[0-9]{4})\].*/\1/' | sort -u)

mapfile -t disk_ids < <(printf '%s\n' "${disk_files[@]}" \
  | sed -E 's#.*/(ADR-[0-9]{4})-.*#\1#' | sort -u)

# 1 + 2: set diff.
missing_in_readme=$(comm -23 <(printf '%s\n' "${disk_ids[@]}") \
                              <(printf '%s\n' "${readme_ids[@]}"))
missing_on_disk=$(comm -13 <(printf '%s\n' "${disk_ids[@]}") \
                            <(printf '%s\n' "${readme_ids[@]}"))

if [[ -n "$missing_in_readme" ]]; then
  echo "[adr-check] ADR files exist on disk but missing from index:" >&2
  echo "$missing_in_readme" >&2
  failed=1
fi
if [[ -n "$missing_on_disk" ]]; then
  echo "[adr-check] README references ADRs that have no file:" >&2
  echo "$missing_on_disk" >&2
  failed=1
fi

# 3: link basename must start with the displayed ID, AND the linked file
#    must exist on disk (relative to docs/adr/).
while IFS= read -r line; do
  if [[ "$line" =~ \[(ADR-[0-9]{4})\]\((ADR-[0-9]{4}-[a-z0-9-]+\.md)\) ]]; then
    disp="${BASH_REMATCH[1]}"
    link="${BASH_REMATCH[2]}"
    case "$link" in
      "$disp"-*) ;;
      *)
        echo "[adr-check] mismatched ADR id and link: '$disp' -> '$link'" >&2
        failed=1
        ;;
    esac
    if [[ ! -f "$adr_dir/$link" ]]; then
      echo "[adr-check] README link '$disp -> $link' has no matching file in $adr_dir/" >&2
      failed=1
    fi
  fi
done < <(grep -E '\[ADR-[0-9]{4}\]\(ADR-[0-9]{4}-' "$readme")

# 4: numeric monotonicity (no duplicate, no gap >1).
prev=0
expected_next=1
for id in "${disk_ids[@]}"; do
  num=$(printf '%s\n' "$id" | sed -E 's/ADR-0*([0-9]+)/\1/')
  num=${num:-0}
  if (( num != expected_next )); then
    echo "[adr-check] ADR numbering gap: expected ADR-$(printf '%04d' "$expected_next") got $id" >&2
    failed=1
  fi
  prev=$num
  expected_next=$((num + 1))
done

# 5: each ADR file must declare a YAML front-matter status.
#    Format: `status: "proposed|accepted|superseded|deprecated|rejected"`.
allowed_status_re='^status:[[:space:]]+"(proposed|accepted|superseded|deprecated|rejected)"'
for f in "${disk_files[@]}"; do
  if ! grep -qE "$allowed_status_re" "$f"; then
    echo "[adr-check] $f missing or invalid front-matter 'status:' line." >&2
    failed=1
  fi
done

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Fix: keep docs/adr/README.md in sync with the files in docs/adr/." >&2
  exit 1
fi

echo "[adr-check] OK (${#disk_ids[@]} ADRs)"
