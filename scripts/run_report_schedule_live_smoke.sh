#!/usr/bin/env bash
# Run the manual M3.1 live scheduled report proof against real services.
#
# This is intentionally NOT part of make ci: it requires live PostgreSQL,
# Temporal, an enabled report workflow schedule, a report-capable worker, and
# the schedule's configured alert source and notification provider wiring.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

require_env DATABASE_URL
require_env TEMPORAL_HOST_PORT
require_env REPORT_WORKFLOW_SCHEDULE_ID
require_env REPORT_WORKFLOW_POLICY_ID
require_env REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT

if ((${#missing[@]} > 0)); then
  printf '[report-schedule-live-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

if [[ -z "${OPENCLARION_LLM_MODEL:-}" ]]; then
  if [[ "${REPORT_LIVE_SMOKE_ASSUME_WORKER_READY:-}" != "1" ]]; then
    cat >&2 <<'EOF'
[report-schedule-live-smoke] cannot confirm worker LLM configuration.
Set OPENCLARION_LLM_MODEL in this shell, or set
REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1 after confirming a report-capable worker
is already running with OPENCLARION_LLM_* configuration.
EOF
    exit 2
  fi
fi

if [[ -z "${OPENCLARION_IM_WEBHOOK_URL:-}" && -z "${OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON:-}" ]]; then
  if [[ "${REPORT_LIVE_SMOKE_ASSUME_WORKER_READY:-}" != "1" ]]; then
    cat >&2 <<'EOF'
[report-schedule-live-smoke] cannot confirm worker notification configuration.
Set OPENCLARION_IM_WEBHOOK_URL for legacy/unbound report delivery, set
OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON for profile-bound report
delivery, or set REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1 after confirming a
report-capable worker is already running with the required notification config.
EOF
    exit 2
  fi
fi

output="$REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT"
wait_timeout="${REPORT_SCHEDULE_WAIT_TIMEOUT:-30m}"
observed_after="${REPORT_SCHEDULE_OBSERVED_AFTER:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"

args=(
  report-schedule-live-smoke
  --schedule-id "$REPORT_WORKFLOW_SCHEDULE_ID"
  --policy-id "$REPORT_WORKFLOW_POLICY_ID"
  --observed-after "$observed_after"
  --wait-timeout "$wait_timeout"
)

if [[ -n "${TEMPORAL_SCHEDULE_ID:-}" ]]; then
  args+=(--temporal-schedule-id "$TEMPORAL_SCHEDULE_ID")
fi

mkdir -p "$(dirname "$output")"

echo "[report-schedule-live-smoke] waiting for a real Temporal Schedule action after $observed_after..." >&2
MANUAL_EVIDENCE_TARGET=report-schedule-live-smoke make --no-print-directory manual-evidence-readiness >/dev/null
go run ./cmd/openclarion "${args[@]}" >"$output"

go run ./scripts/report_schedule_live_smoke_output "$output"

echo "[report-schedule-live-smoke] OK - live smoke output: $output" >&2
