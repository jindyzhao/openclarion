#!/usr/bin/env bash
# Shared private env-file boundary for manual operator scripts.
#
# Callers intentionally source operator-owned shell assignment files. The file
# must be operator-private: outside this repository, or under the repo-local
# ignored .openclarion-private/ directory, and not readable by group/other.

openclarion_load_private_env_file() {
  local label="$1"
  local root_dir="$2"
  local env_file="$3"

  if [[ -z "$env_file" ]]; then
    return 0
  fi

  if [[ -L "$env_file" || ! -f "$env_file" ]]; then
    printf '[%s] env file must be a direct regular file\n' "$label" >&2
    return 2
  fi

  local env_dir
  local env_base
  local env_file_abs
  if ! env_dir="$(cd "$(dirname "$env_file")" && pwd -P)"; then
    printf '[%s] env file directory must be accessible\n' "$label" >&2
    return 2
  fi
  env_base="$(basename "$env_file")"
  env_file_abs="$env_dir/$env_base"

  case "$env_file_abs" in
    "$root_dir"/*|"$root_dir")
      if ! openclarion_private_env_file_allowed_in_repo "$label" "$root_dir" "$env_file_abs"; then
        return 2
      fi
      ;;
  esac

  local owner_uid
  owner_uid="$(stat -c '%u' "$env_file_abs")"
  if [[ "$owner_uid" != "$(id -u)" ]]; then
    printf '[%s] env file must be owned by the current user\n' "$label" >&2
    return 2
  fi

  local mode
  local mode_last3
  mode="$(stat -c '%a' "$env_file_abs")"
  mode_last3="${mode: -3}"
  if (( (8#$mode_last3 & 077) != 0 )); then
    printf '[%s] env file must not be readable, writable, or executable by group/other\n' "$label" >&2
    return 2
  fi

  set -o allexport
  # shellcheck source=/dev/null
  if ! source "$env_file_abs"; then
    set +o allexport
    return 2
  fi
  set +o allexport
}

openclarion_capture_exported_env_overrides() {
  declare -g -A __openclarion_private_env_override_values=()
  declare -g -A __openclarion_private_env_override_set=()

  local entry
  local name
  while IFS= read -r -d '' entry; do
    name="${entry%%=*}"
    if openclarion_private_env_name_valid "$name"; then
      __openclarion_private_env_override_values["$name"]="${!name}"
      __openclarion_private_env_override_set["$name"]="1"
    fi
  done < <(env -0)
}

openclarion_restore_exported_env_overrides() {
  declare -g -A __openclarion_private_env_override_values
  declare -g -A __openclarion_private_env_override_set

  local name
  for name in "${!__openclarion_private_env_override_set[@]}"; do
    export "$name=${__openclarion_private_env_override_values[$name]}"
  done
}

openclarion_private_env_name_valid() {
  [[ "$1" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]
}

openclarion_private_env_file_allowed_in_repo() {
  local label="$1"
  local root_dir="$2"
  local env_file_abs="$3"
  local rel

  rel="${env_file_abs#"$root_dir"/}"
  case "$rel" in
    .openclarion-private/*)
      ;;
    *)
      printf '[%s] env file must live outside the repository or under .openclarion-private/\n' "$label" >&2
      return 2
      ;;
  esac

  if ! command -v git >/dev/null 2>&1 || ! git -C "$root_dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    printf '[%s] repo-local env file requires git ignore verification\n' "$label" >&2
    return 2
  fi
  if git -C "$root_dir" ls-files --error-unmatch -- "$rel" >/dev/null 2>&1; then
    printf '[%s] repo-local env file must not be tracked by git\n' "$label" >&2
    return 2
  fi
  if ! git -C "$root_dir" check-ignore -q -- "$rel"; then
    printf '[%s] repo-local env file must be ignored by git\n' "$label" >&2
    return 2
  fi
}
