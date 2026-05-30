#!/usr/bin/env bash
# Validate that docs/adr/README.md is consistent with files on disk.
#
# Checks:
#   1. Every ADR file ADR-NNNN-*.md is referenced by README.md.
#   2. Every README.md row points to a file that exists.
#   3. Every README.md row's link basename matches the displayed ID.
#   4. ADR ids in README.md are unique and monotonic (no gaps).
#   5. Every ADR file has valid YAML front matter.
#   6. Every ADR file has Status: <one of accepted statuses>.
#   7. Every ADR file has a non-empty Consequences section.
#   8. On PR/base-aware runs, previously accepted ADR bodies are immutable.
#
# This is a string-level lint; it does not parse markdown ASTs.

set -euo pipefail

cd "$(dirname "$0")/.."

readme="docs/adr/README.md"
adr_dir="docs/adr"

require_direct_directory() {
  local path="$1"
  if [[ -L "$path" ]]; then
    echo "[adr-check] $path must be a directory, not a symlink." >&2
    return 1
  fi
  if [[ ! -d "$path" ]]; then
    echo "[adr-check] $path must be a directory." >&2
    return 1
  fi
}

require_regular_file() {
  local path="$1"
  if [[ -L "$path" ]]; then
    echo "[adr-check] $path must be a regular file, not a symlink." >&2
    return 1
  fi
  if [[ ! -f "$path" ]]; then
    echo "[adr-check] $path must be a regular file." >&2
    return 1
  fi
}

failed=0

if ! require_direct_directory "$adr_dir"; then
  exit 1
fi
if ! require_regular_file "$readme"; then
  exit 1
fi

mapfile -t invalid_adr_files < <(find "$adr_dir" -maxdepth 1 \
  -name 'ADR-*.md' ! -type f | sort)
for f in "${invalid_adr_files[@]}"; do
  if ! require_regular_file "$f"; then
    failed=1
  fi
done

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
    if ! require_regular_file "$adr_dir/$link"; then
      echo "[adr-check] README link '$disp -> $link' must point to a regular file in $adr_dir/" >&2
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

strip_yaml_scalar_quotes() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  if [[ "$value" =~ ^\"(.*)\"$ ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  else
    printf '%s' "$value"
  fi
}

declare -A adr_file_by_id=()
declare -A adr_status_by_id=()
declare -A adr_supersedes_by_id=()
declare -A adr_superseded_by_id=()

validate_front_matter_schema() {
  local file="$1"
  local expected_id="$2"
  local line line_number=0 closed=0
  declare -A fields=()
  local required_keys=(id title status date deciders consulted informed)
  local allowed_key_re='^(id|title|status|date|deciders|consulted|informed|supersedes|superseded_by)$'

  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    if [[ "$line_number" -eq 1 ]]; then
      if [[ "$line" != "---" ]]; then
        echo "[adr-check] $file missing opening YAML front-matter delimiter." >&2
        return 1
      fi
      continue
    fi

    if [[ "$line" == "---" ]]; then
      closed=1
      break
    fi

    if [[ -z "$line" ]]; then
      continue
    fi

    if [[ ! "$line" =~ ^([a-zA-Z_]+):[[:space:]]*(.*)$ ]]; then
      echo "[adr-check] $file:$line_number invalid front-matter line: $line" >&2
      return 1
    fi

    local key="${BASH_REMATCH[1]}"
    local value="${BASH_REMATCH[2]}"
    if [[ ! "$key" =~ $allowed_key_re ]]; then
      echo "[adr-check] $file unknown front-matter key '$key'." >&2
      return 1
    fi
    if [[ -n "${fields[$key]+x}" ]]; then
      echo "[adr-check] $file duplicate front-matter key '$key'." >&2
      return 1
    fi
    fields["$key"]="$value"
  done < "$file"

  if [[ "$closed" -ne 1 ]]; then
    echo "[adr-check] $file missing closing YAML front-matter delimiter." >&2
    return 1
  fi

  for key in "${required_keys[@]}"; do
    if [[ -z "${fields[$key]+x}" ]]; then
      echo "[adr-check] $file missing front-matter '$key'." >&2
      return 1
    fi
  done

  local id title status date deciders consulted informed supersedes superseded_by
  id="$(strip_yaml_scalar_quotes "${fields[id]}")"
  title="$(strip_yaml_scalar_quotes "${fields[title]}")"
  status="$(strip_yaml_scalar_quotes "${fields[status]}")"
  date="$(strip_yaml_scalar_quotes "${fields[date]}")"
  deciders="$(strip_yaml_scalar_quotes "${fields[deciders]}")"
  consulted="$(strip_yaml_scalar_quotes "${fields[consulted]}")"
  informed="$(strip_yaml_scalar_quotes "${fields[informed]}")"
  supersedes=""
  superseded_by=""
  if [[ -n "${fields[supersedes]+x}" ]]; then
    supersedes="$(strip_yaml_scalar_quotes "${fields[supersedes]}")"
  fi
  if [[ -n "${fields[superseded_by]+x}" ]]; then
    superseded_by="$(strip_yaml_scalar_quotes "${fields[superseded_by]}")"
  fi

  if [[ "$id" != "$expected_id" ]]; then
    echo "[adr-check] $file front-matter id '$id' does not match file id '$expected_id'." >&2
    return 1
  fi
  if [[ ! "$status" =~ ^(proposed|accepted|superseded|deprecated|rejected)$ ]]; then
    echo "[adr-check] $file front-matter status '$status' is not allowed." >&2
    return 1
  fi
  if [[ ! "$date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    echo "[adr-check] $file front-matter date '$date' must be YYYY-MM-DD." >&2
    return 1
  fi
  for key_value in "$deciders" "$consulted" "$informed"; do
    if [[ ! "$key_value" =~ ^\[.*\]$ ]]; then
      echo "[adr-check] $file front-matter participant fields must be YAML arrays." >&2
      return 1
    fi
  done

  local h1
  h1="$(grep -m1 -E '^# ADR-[0-9]{4}: .+' "$file" || true)"
  if [[ ! "$h1" =~ ^#[[:space:]]+(ADR-[0-9]{4}):[[:space:]]+(.+)$ ]]; then
    echo "[adr-check] $file missing H1 '# ADR-NNNN: Title'." >&2
    return 1
  fi
  if [[ "${BASH_REMATCH[1]}" != "$id" ]]; then
    echo "[adr-check] $file H1 id '${BASH_REMATCH[1]}' does not match front-matter id '$id'." >&2
    return 1
  fi
  if [[ "${BASH_REMATCH[2]}" != "$title" ]]; then
    echo "[adr-check] $file front-matter title '$title' does not match H1 title '${BASH_REMATCH[2]}'." >&2
    return 1
  fi

  adr_file_by_id["$id"]="$file"
  adr_status_by_id["$id"]="$status"
  adr_supersedes_by_id["$id"]="$supersedes"
  adr_superseded_by_id["$id"]="$superseded_by"
}

for f in "${disk_files[@]}"; do
  expected_id="$(printf '%s\n' "$f" | sed -E 's#.*/(ADR-[0-9]{4})-.*#\1#')"
  if ! validate_front_matter_schema "$f" "$expected_id"; then
    failed=1
  fi
done

parse_adr_ref_list() {
  local file="$1"
  local key="$2"
  local raw="$3"
  local -n result="$4"
  result=()

  raw="${raw#"${raw%%[![:space:]]*}"}"
  raw="${raw%"${raw##*[![:space:]]}"}"
  if [[ -z "$raw" || "$raw" == "[]" ]]; then
    return 0
  fi
  if [[ "$raw" =~ ^\[(.*)\]$ ]]; then
    raw="${BASH_REMATCH[1]}"
  fi

  local item
  IFS=',' read -r -a raw_items <<< "$raw"
  for item in "${raw_items[@]}"; do
    item="$(strip_yaml_scalar_quotes "$item")"
    item="${item#"${item%%[![:space:]]*}"}"
    item="${item%"${item##*[![:space:]]}"}"
    if [[ -z "$item" ]]; then
      continue
    fi
    if [[ ! "$item" =~ ^ADR-[0-9]{4}$ ]]; then
      echo "[adr-check] $file front-matter '$key' reference '$item' must be ADR-NNNN." >&2
      return 1
    fi
    result+=("$item")
  done
}

contains_ref() {
  local needle="$1"
  shift
  local ref
  for ref in "$@"; do
    if [[ "$ref" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

front_matter_value() {
  local file="$1"
  local key="$2"
  local line line_number=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    if [[ "$line_number" -eq 1 && "$line" != "---" ]]; then
      return 1
    fi
    if [[ "$line_number" -gt 1 && "$line" == "---" ]]; then
      return 1
    fi
    if [[ "$line" =~ ^${key}:[[:space:]]*(.*)$ ]]; then
      strip_yaml_scalar_quotes "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$file"
  return 1
}

adr_body_without_front_matter() {
  local file="$1"
  awk '
    NR == 1 && $0 == "---" {
      in_front_matter = 1
      next
    }
    in_front_matter && $0 == "---" {
      in_front_matter = 0
      next
    }
    !in_front_matter {
      print
    }
  ' "$file"
}

for id in "${disk_ids[@]}"; do
  file="${adr_file_by_id[$id]}"
  status="${adr_status_by_id[$id]}"
  declare -a supersedes_refs=()
  declare -a superseded_by_refs=()

  if ! parse_adr_ref_list "$file" "supersedes" "${adr_supersedes_by_id[$id]}" supersedes_refs; then
    failed=1
    continue
  fi
  if ! parse_adr_ref_list "$file" "superseded_by" "${adr_superseded_by_id[$id]}" superseded_by_refs; then
    failed=1
    continue
  fi

  if (( ${#superseded_by_refs[@]} > 0 )) && [[ "$status" != "superseded" ]]; then
    echo "[adr-check] $file declares superseded_by but status is '$status', expected 'superseded'." >&2
    failed=1
  fi
  if [[ "$status" == "superseded" ]] && (( ${#superseded_by_refs[@]} == 0 )); then
    echo "[adr-check] $file has status 'superseded' but no superseded_by back-pointer." >&2
    failed=1
  fi

  for ref in "${supersedes_refs[@]}"; do
    if [[ -z "${adr_file_by_id[$ref]+x}" ]]; then
      echo "[adr-check] $file supersedes '$ref', but that ADR does not exist." >&2
      failed=1
      continue
    fi
    if [[ "${adr_status_by_id[$ref]}" != "superseded" ]]; then
      echo "[adr-check] $file supersedes '$ref', but ${adr_file_by_id[$ref]} status is '${adr_status_by_id[$ref]}', expected 'superseded'." >&2
      failed=1
    fi

    declare -a target_superseded_by_refs=()
    if ! parse_adr_ref_list "${adr_file_by_id[$ref]}" "superseded_by" "${adr_superseded_by_id[$ref]}" target_superseded_by_refs; then
      failed=1
      continue
    fi
    if ! contains_ref "$id" "${target_superseded_by_refs[@]}"; then
      echo "[adr-check] $file supersedes '$ref', but ${adr_file_by_id[$ref]} does not declare superseded_by: $id." >&2
      failed=1
    fi
  done
done

resolve_adr_base_ref() {
  local -a candidates=()
  if [[ -n "${ADR_BASE_REF:-}" ]]; then
    candidates+=("$ADR_BASE_REF" "origin/$ADR_BASE_REF")
  fi
  local ref
  for ref in "${candidates[@]}"; do
    if git rev-parse --quiet --verify "$ref^{tree}" >/dev/null; then
      printf '%s' "$ref"
      return 0
    fi
  done
  return 1
}

check_accepted_adr_body_immutability() {
  local base_ref="$1"
  local tmpdir="$2"
  local -a base_adr_files=()
  mapfile -t base_adr_files < <(git ls-tree -r --name-only "$base_ref" "$adr_dir" \
    | grep -E '^docs/adr/ADR-[0-9]{4}-[a-z0-9-]+\.md$' || true)

  local base_path base_file current_body base_body base_status
  for base_path in "${base_adr_files[@]}"; do
    base_file="$tmpdir/$(basename "$base_path")"
    git show "$base_ref:$base_path" >"$base_file"
    base_status="$(front_matter_value "$base_file" "status" || true)"
    if [[ "$base_status" != "accepted" ]]; then
      continue
    fi
    if ! require_regular_file "$base_path"; then
      echo "[adr-check] accepted ADR '$base_path' existed at $base_ref but is missing or not a regular file in the current tree; supersede it with a new ADR instead of deleting or replacing it." >&2
      failed=1
      continue
    fi
    base_body="$tmpdir/$(basename "$base_path").base.body"
    current_body="$tmpdir/$(basename "$base_path").current.body"
    adr_body_without_front_matter "$base_file" >"$base_body"
    adr_body_without_front_matter "$base_path" >"$current_body"
    if ! cmp -s "$base_body" "$current_body"; then
      echo "[adr-check] accepted ADR body changed: $base_path. Create a new superseding ADR instead of editing the accepted decision body." >&2
      failed=1
    fi
  done
}

if [[ -n "${ADR_BASE_REF:-}" ]]; then
  if base_ref="$(resolve_adr_base_ref)"; then
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT
    check_accepted_adr_body_immutability "$base_ref" "$tmpdir"
  else
    echo "[adr-check] ADR_BASE_REF '$ADR_BASE_REF' could not be resolved. Fetch the PR base ref before running adr-check." >&2
    failed=1
  fi
fi

has_consequences_section() {
  local file="$1"
  awk '
    /^##[[:space:]]+Consequences[[:space:]]*$/ || /^###[[:space:]]+Consequences[[:space:]]*$/ {
      found = 1
      in_section = 1
      next
    }
    in_section && /^##[[:space:]#]/ {
      in_section = 0
    }
    in_section && /[^[:space:]]/ {
      saw_body = 1
    }
    END {
      exit(found && saw_body ? 0 : 1)
    }
  ' "$file"
}

for f in "${disk_files[@]}"; do
  if ! has_consequences_section "$f"; then
    echo "[adr-check] $f missing a non-empty 'Consequences' section." >&2
    failed=1
  fi
done

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Fix: keep docs/adr/README.md in sync with the files in docs/adr/, keep ADR front matter schema-valid, include a non-empty Consequences section in every ADR, close supersession references, and do not edit accepted ADR bodies in place." >&2
  exit 1
fi

echo "[adr-check] OK (${#disk_ids[@]} ADRs)"
