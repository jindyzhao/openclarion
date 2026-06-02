#!/usr/bin/env bash
# Reject mutable version pins in dependency manifests and Docker base images.
#
# Background:
#   - docs/design/DEPENDENCIES.md: "CI must reject the literal string
#     `latest` in `go.mod` and `package.json` for first-party dependencies."
#   - docs/design/DEPENDENCIES.md: sandbox/runtime and production container
#     images must be pinned to immutable digests once Dockerfiles land.
#   - ADR-0007: oapi-codegen-exp must be pinned by commit hash.
#
# Behavior:
#   - go.mod: rejects any `module v0.0.0-...latest` style or literal `latest`.
#     Go modules do not actually allow `latest` in go.mod (it gets resolved
#     at `go get` time), so this is a defense-in-depth check on text content.
#   - go.mod: requires committed `replace` directives to be explicitly
#     allowlisted in docs/design/DEPENDENCIES.md.
#   - go.mod: requires every external `tool` directive path to be backed by
#     a concrete `require` version pin for the module that provides the tool.
#   - root go.mod: requires critical first-import modules to be direct
#     concrete version pins, and rejects `replace` directives for them.
#   - package.json: rejects any dependency value that is exactly "latest" or
#     starts with a semver range operator (`^` or `~`). First-party manifests
#     must use concrete pins; transitive ranges remain confined to lockfiles.
#   - package.json + GitHub Actions: requires @types/node major versions to
#     match the single numeric actions/setup-node major used by CI.
#   - Dockerfile: rejects external base images that are not pinned with an
#     `@sha256:<digest>` reference. `scratch` and previously declared build
#     stages are allowed because they do not pull a mutable external base.
#   - No-op when none of these files exists yet.

set -euo pipefail

cd "$(dirname "$0")/.."

failed=0

# -------- go.mod / go.sum (all first-party Go modules) --------
mapfile -t go_mod_files < <(find . \
  -path ./vendor -prune -o \
  -path './**/vendor' -prune -o \
  -name 'go.mod' -print 2>/dev/null)

for mod in "${go_mod_files[@]}"; do
  if grep -nE '\blatest\b' "$mod"; then
    echo "[forbidden-latest] $mod must not reference 'latest'." >&2
    failed=1
  fi
done

mapfile -t go_sum_files < <(find . \
  -path ./vendor -prune -o \
  -path './**/vendor' -prune -o \
  -name 'go.sum' -print 2>/dev/null)

for sum in "${go_sum_files[@]}"; do
  if grep -nE '\blatest\b' "$sum"; then
    echo "[forbidden-latest] $sum must not reference 'latest'." >&2
    failed=1
  fi
done

# -------- critical first-import Go module pins (root module only) --------
critical_go_modules=(
  "entgo.io/ent"
  "go.temporal.io/sdk"
  "go.opentelemetry.io/otel"
  "go.opentelemetry.io/otel/sdk"
  "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
  "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)
concrete_go_version_re='^v[0-9]+[.][0-9]+[.][0-9]+([-.+][0-9A-Za-z.-]+)?$'
replace_allowlist_file="docs/design/DEPENDENCIES.md"

reject_symlink_ancestors() {
  local file="$1"
  local prefix="$2"
  local dir=""
  local part=""
  local path_part=""
  local -a parts=()

  if [[ "$file" != */* ]]; then
    return 0
  fi
  dir="${file%/*}"

  IFS='/' read -r -a parts <<< "$dir"
  for part in "${parts[@]}"; do
    if [[ -z "$part" || "$part" == "." ]]; then
      continue
    fi
    if [[ -z "$path_part" ]]; then
      path_part="$part"
    else
      path_part="$path_part/$part"
    fi
    if [[ -L "$path_part" ]]; then
      echo "[$prefix] $file parent directory $path_part must not be a symlink" >&2
      return 1
    fi
    if [[ -e "$path_part" && ! -d "$path_part" ]]; then
      echo "[$prefix] $file parent directory $path_part must be a directory" >&2
      return 1
    fi
  done
  return 0
}

is_critical_go_module() {
  local module="$1"
  local critical
  for critical in "${critical_go_modules[@]}"; do
    if [[ "$module" == "$critical" ]]; then
      return 0
    fi
  done
  return 1
}

check_tool_require_pins() {
  local mod="$1"
  local module_path=""
  local line_number=0
  local in_require_block=0
  local in_tool_block=0
  local line=""
  local line_without_comment=""
  local trimmed=""
  local require_line=""
  local tool_line=""
  local required_module=""
  local version=""
  local rest=""
  local tool_path=""
  local entry=""
  local best_module=""
  local best_len=0

  declare -A require_versions=()
  local -a tool_entries=()

  while IFS= read -r line || [[ -n "$line" ]]; do
    ((line_number += 1))
    line_without_comment="${line%%//*}"
    trimmed="${line_without_comment#"${line_without_comment%%[![:space:]]*}"}"
    trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"

    if [[ -z "$trimmed" ]]; then
      continue
    fi

    if [[ "$trimmed" =~ ^module[[:space:]]+([^[:space:]]+)$ ]]; then
      module_path="${BASH_REMATCH[1]}"
      continue
    fi

    if [[ "$trimmed" == "require (" ]]; then
      in_require_block=1
      continue
    fi
    if [[ "$trimmed" == "tool (" ]]; then
      in_tool_block=1
      continue
    fi
    if [[ "$trimmed" == ")" ]]; then
      in_require_block=0
      in_tool_block=0
      continue
    fi

    require_line=""
    if [[ "$in_require_block" -eq 1 ]]; then
      require_line="$trimmed"
    elif [[ "$trimmed" =~ ^require[[:space:]]+(.+)$ ]]; then
      require_line="${BASH_REMATCH[1]}"
    fi
    if [[ -n "$require_line" ]]; then
      read -r required_module version rest <<< "$require_line"
      if [[ -n "${required_module:-}" && -n "${version:-}" ]]; then
        require_versions["$required_module"]="$version"
      fi
    fi

    tool_line=""
    if [[ "$in_tool_block" -eq 1 ]]; then
      tool_line="$trimmed"
    elif [[ "$trimmed" =~ ^tool[[:space:]]+(.+)$ ]]; then
      tool_line="${BASH_REMATCH[1]}"
    fi
    if [[ -n "$tool_line" ]]; then
      read -r tool_path rest <<< "$tool_line"
      if [[ -n "$tool_path" ]]; then
        tool_entries+=("$line_number:$tool_path")
      fi
    fi
  done < "$mod"

  for entry in "${tool_entries[@]}"; do
    line_number="${entry%%:*}"
    tool_path="${entry#*:}"

    if [[ "$tool_path" == ./* || "$tool_path" == ../* ]]; then
      continue
    fi
    if [[ -n "$module_path" && ( "$tool_path" == "$module_path" || "$tool_path" == "$module_path/"* ) ]]; then
      continue
    fi

    best_module=""
    best_len=0
    for required_module in "${!require_versions[@]}"; do
      if [[ "$tool_path" == "$required_module" || "$tool_path" == "$required_module/"* ]]; then
        if (( ${#required_module} > best_len )); then
          best_module="$required_module"
          best_len="${#required_module}"
        fi
      fi
    done

    if [[ -z "$best_module" ]]; then
      echo "[forbidden-latest] $mod:$line_number tool directive $tool_path must be backed by a concrete require pin for its module." >&2
      failed=1
      continue
    fi

    version="${require_versions[$best_module]}"
    if [[ ! "$version" =~ $concrete_go_version_re ]]; then
      echo "[forbidden-latest] $mod:$line_number tool directive $tool_path resolves to $best_module, which must use a concrete semantic or pseudo-version pin, got '$version'." >&2
      failed=1
    fi
  done
}

check_replace_allowlist() {
  local mod="$1"
  local line_number="$2"
  local replace_line="$3"
  local -a replace_tokens=()
  local old_module=""
  local target=""
  local i

  read -r -a replace_tokens <<< "$replace_line"
  old_module="${replace_tokens[0]:-}"
  for ((i = 1; i < ${#replace_tokens[@]}; i++)); do
    if [[ "${replace_tokens[$i]}" == "=>" ]]; then
      target="${replace_tokens[$((i + 1))]:-}"
      break
    fi
  done

  if [[ -z "$old_module" || -z "$target" ]]; then
    echo "[forbidden-latest] $mod:$line_number contains malformed replace directive: $replace_line" >&2
    failed=1
    return
  fi

  local marker="replace-allow: $old_module => $target"
  local allowlist_line=""
  local expires_at=""
  local expires_normalized=""
  if ! reject_symlink_ancestors "$replace_allowlist_file" "forbidden-latest"; then
    failed=1
    return
  fi
  if [[ -L "$replace_allowlist_file" ]]; then
    echo "[forbidden-latest] $replace_allowlist_file must be a regular file, not a symlink" >&2
    failed=1
    return
  fi
  if [[ -e "$replace_allowlist_file" && ! -f "$replace_allowlist_file" ]]; then
    echo "[forbidden-latest] $replace_allowlist_file must be a regular file" >&2
    failed=1
    return
  fi
  if [[ -e "$replace_allowlist_file" ]]; then
    allowlist_line="$(grep -F "$marker" "$replace_allowlist_file" | head -n 1 || true)"
  fi

  if [[ -z "$allowlist_line" ]]; then
    echo "[forbidden-latest] $mod:$line_number replace directive must be documented in $replace_allowlist_file with '$marker; owner: <owner>; expires: YYYY-MM-DD; reason: <reason>'." >&2
    failed=1
    return
  fi
  local owner_re='owner:[[:space:]]*[^[:space:];,]+'
  local expires_re='expires:[[:space:]]*([0-9]{4}-[0-9]{2}-[0-9]{2})'
  if [[ ! "$allowlist_line" =~ $owner_re ]]; then
    echo "[forbidden-latest] $replace_allowlist_file replace allowlist entry for '$marker' must include owner: <owner>." >&2
    failed=1
  fi
  if [[ ! "$allowlist_line" =~ $expires_re ]]; then
    echo "[forbidden-latest] $replace_allowlist_file replace allowlist entry for '$marker' must include expires: YYYY-MM-DD." >&2
    failed=1
  else
    expires_at="${BASH_REMATCH[1]}"
    if ! expires_normalized="$(date -u -d "$expires_at" +%F 2>/dev/null)" || [[ "$expires_normalized" != "$expires_at" ]]; then
      echo "[forbidden-latest] $replace_allowlist_file replace allowlist entry for '$marker' has invalid expires date $expires_at." >&2
      failed=1
    elif [[ "$expires_at" < "$(date -u +%F)" ]]; then
      echo "[forbidden-latest] $replace_allowlist_file replace allowlist entry for '$marker' expired on $expires_at." >&2
      failed=1
    fi
  fi
}

for mod in "${go_mod_files[@]}"; do
  check_tool_require_pins "$mod"

  in_replace_block=0
  line_number=0

  while IFS= read -r line || [[ -n "$line" ]]; do
    ((line_number += 1))
    line_without_comment="${line%%//*}"
    trimmed="${line_without_comment#"${line_without_comment%%[![:space:]]*}"}"
    trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"

    if [[ "$trimmed" == "replace (" ]]; then
      in_replace_block=1
      continue
    fi
    if [[ "$trimmed" == ")" ]]; then
      in_replace_block=0
      continue
    fi
    if [[ -z "$trimmed" ]]; then
      continue
    fi

    replace_line=""
    if [[ "$in_replace_block" -eq 1 ]]; then
      replace_line="$trimmed"
    elif [[ "$trimmed" =~ ^replace[[:space:]]+(.+)$ ]]; then
      replace_line="${BASH_REMATCH[1]}"
    fi

    if [[ -n "$replace_line" ]]; then
      check_replace_allowlist "$mod" "$line_number" "$replace_line"
    fi
  done < "$mod"
done

if [[ -f go.mod ]]; then
  declare -A critical_versions=()
  declare -A critical_indirect=()
  in_require_block=0
  in_replace_block=0

  while IFS= read -r line || [[ -n "$line" ]]; do
    trimmed="${line#"${line%%[![:space:]]*}"}"
    trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"

    if [[ "$trimmed" == "require (" ]]; then
      in_require_block=1
      continue
    fi
    if [[ "$trimmed" == "replace (" ]]; then
      in_replace_block=1
      continue
    fi
    if [[ "$trimmed" == ")" ]]; then
      in_require_block=0
      in_replace_block=0
      continue
    fi

    require_line=""
    if [[ "$in_require_block" -eq 1 ]]; then
      require_line="$trimmed"
    elif [[ "$trimmed" =~ ^require[[:space:]]+(.+)$ ]]; then
      require_line="${BASH_REMATCH[1]}"
    fi

    if [[ -n "$require_line" ]]; then
      read -r module version rest <<< "$require_line"
      if is_critical_go_module "$module"; then
        critical_versions["$module"]="$version"
        if [[ "$require_line" =~ //[[:space:]]*indirect ]]; then
          critical_indirect["$module"]=1
        fi
      fi
    fi

    replace_line=""
    if [[ "$in_replace_block" -eq 1 ]]; then
      replace_line="$trimmed"
    elif [[ "$trimmed" =~ ^replace[[:space:]]+(.+)$ ]]; then
      replace_line="${BASH_REMATCH[1]}"
    fi

    if [[ -n "$replace_line" ]]; then
      read -r replaced_module _replace_arrow _replace_target _replace_version <<< "$replace_line"
      if is_critical_go_module "$replaced_module"; then
        echo "[forbidden-latest] go.mod must not replace critical first-import module $replaced_module." >&2
        failed=1
      fi
    fi
  done < go.mod

  for critical in "${critical_go_modules[@]}"; do
    if [[ -z "${critical_versions[$critical]+x}" ]]; then
      echo "[forbidden-latest] go.mod must directly require critical first-import module $critical." >&2
      failed=1
      continue
    fi
    if [[ -n "${critical_indirect[$critical]+x}" ]]; then
      echo "[forbidden-latest] go.mod critical first-import module $critical must be a direct require, not // indirect." >&2
      failed=1
    fi
    if [[ ! "${critical_versions[$critical]}" =~ $concrete_go_version_re ]]; then
      echo "[forbidden-latest] go.mod critical first-import module $critical must use a concrete semantic or pseudo-version pin, got '${critical_versions[$critical]}'." >&2
      failed=1
    fi
  done
fi

# -------- package.json (any depth, excluding node_modules) --------
mapfile -t pkg_files < <(find . \
  -path ./node_modules -prune -o \
  -path ./vendor -prune -o \
  -path './**/node_modules' -prune -o \
  -name 'package.json' -print 2>/dev/null)

for pkg in "${pkg_files[@]}"; do
  # Match `"some-dep": "latest"` (allowing whitespace variants).
  if grep -nE '"[^"]+"[[:space:]]*:[[:space:]]*"latest"' "$pkg"; then
    echo "[forbidden-latest] $pkg must not pin any dependency to 'latest'." >&2
    failed=1
  fi
  # Match direct dependency ranges like `"some-dep": "^1.2.3"` or "~1.2.3".
  # This intentionally checks package.json only, not lockfiles: lockfiles are
  # the place where transitive ranges resolve to concrete package versions.
  if grep -nE '"[^"]+"[[:space:]]*:[[:space:]]*"[~^][^"]*"' "$pkg"; then
    echo "[forbidden-latest] $pkg must pin dependencies to exact versions, not ^/~ ranges." >&2
    failed=1
  fi
done

declare -A ci_node_majors=()
if [[ -d .github/workflows ]]; then
  while IFS= read -r major; do
    if [[ -n "$major" ]]; then
      ci_node_majors["$major"]=1
    fi
  done < <(find .github/workflows \
    -path './**/node_modules' -prune -o \
    \( -name '*.yml' -o -name '*.yaml' \) -exec sed -nE "s/^[[:space:]]*node-version:[[:space:]]*[\"']?([0-9]+)([.][0-9]+){0,2}[\"']?[[:space:]]*(#.*)?$/\1/p" {} + 2>/dev/null)
fi

ci_node_major=""
ci_node_major_count=0
for major in "${!ci_node_majors[@]}"; do
  ci_node_major="$major"
  ((ci_node_major_count += 1))
done

for pkg in "${pkg_files[@]}"; do
  while IFS=: read -r line_number node_types_version || [[ -n "${line_number:-}" ]]; do
    if [[ -z "${line_number:-}" || -z "${node_types_version:-}" ]]; then
      continue
    fi

    if [[ $ci_node_major_count -ne 1 ]]; then
      echo "[forbidden-latest] $pkg:$line_number @types/node requires exactly one numeric actions/setup-node node-version major in .github/workflows, found $ci_node_major_count." >&2
      failed=1
      continue
    fi

    if [[ ! "$node_types_version" =~ ^([0-9]+)[.][0-9]+[.][0-9]+([-.+][0-9A-Za-z.-]+)?$ ]]; then
      echo "[forbidden-latest] $pkg:$line_number @types/node must use a concrete semantic version, got '$node_types_version'." >&2
      failed=1
      continue
    fi

    node_types_major="${BASH_REMATCH[1]}"
    if [[ "$node_types_major" != "$ci_node_major" ]]; then
      echo "[forbidden-latest] $pkg:$line_number @types/node major $node_types_major must match CI Node.js major $ci_node_major." >&2
      failed=1
    fi
  done < <(grep -nE '"@types/node"[[:space:]]*:[[:space:]]*"[^"]+"' "$pkg" \
    | sed -E 's/^([0-9]+):.*"@types\/node"[[:space:]]*:[[:space:]]*"([^"]+)".*$/\1:\2/')
done

# -------- Dockerfile base image pins --------
mapfile -t docker_files < <(find . \
  -path ./.git -prune -o \
  -path ./node_modules -prune -o \
  -path ./vendor -prune -o \
  -path './**/node_modules' -prune -o \
  -path './**/vendor' -prune -o \
  \( -name 'Dockerfile' -o -name 'Dockerfile.*' -o -name '*.Dockerfile' \) -print 2>/dev/null)

digest_ref_re='@sha256:[[:xdigit:]]{64}$'

for dockerfile in "${docker_files[@]}"; do
  declare -A stage_names=()
  line_number=0

  while IFS= read -r line || [[ -n "$line" ]]; do
    ((line_number += 1))
    line_without_comment="${line%%#*}"
    if [[ ! "$line_without_comment" =~ [^[:space:]] ]]; then
      continue
    fi

    read -r -a tokens <<< "$line_without_comment"
    instruction="${tokens[0],,}"
    if [[ "$instruction" != "from" ]]; then
      continue
    fi

    image_index=1
    while [[ "${tokens[$image_index]:-}" == --* ]]; do
      ((image_index += 1))
    done

    image_ref="${tokens[$image_index]:-}"
    if [[ -z "$image_ref" ]]; then
      printf '%s:%d: %s\n' "$dockerfile" "$line_number" "$line" >&2
      echo "[forbidden-latest] malformed Dockerfile FROM instruction without an image." >&2
      failed=1
      continue
    fi

    image_ref_lc="${image_ref,,}"
    if [[ "$image_ref_lc" != "scratch" \
      && -z "${stage_names[$image_ref]+x}" \
      && ! "$image_ref" =~ $digest_ref_re ]]; then
      printf '%s:%d: %s\n' "$dockerfile" "$line_number" "$line" >&2
      echo "[forbidden-latest] external Docker base image must be pinned to @sha256:<digest> (scratch and previous build stages are allowed): $image_ref" >&2
      failed=1
    fi

    for ((i = image_index + 1; i < ${#tokens[@]} - 1; i++)); do
      if [[ "${tokens[$i],,}" == "as" ]]; then
        stage_names["${tokens[$((i + 1))]}"]=1
        break
      fi
    done
  done < "$dockerfile"
done

if [[ ${#pkg_files[@]} -eq 0 && ${#go_mod_files[@]} -eq 0 && ${#docker_files[@]} -eq 0 ]]; then
  echo "[forbidden-latest] no go.mod, package.json, or Dockerfile yet; skipping."
  exit 0
fi

if [[ $failed -ne 0 ]]; then
  echo "" >&2
  echo "Justification: docs/design/DEPENDENCIES.md forbids mutable dependency pins for first-party manifests and Docker runtime images." >&2
  echo "Fix: pin dependencies to a concrete version, document temporary Go replace directives with owner/expiry, and pin external Docker base images to @sha256:<digest>." >&2
  exit 1
fi

echo "[forbidden-latest] OK"
