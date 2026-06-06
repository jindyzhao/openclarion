#!/usr/bin/env bash
# Run the manual M3.1 live report smoke check through persisted report workflow
# policy configuration.
#
# This is intentionally NOT part of make ci: it requires live PostgreSQL,
# Temporal, an enabled report workflow policy, a report-capable worker, and the
# policy's configured alert source provider.

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
require_env REPORT_WORKFLOW_POLICY_ID
require_env REPORT_WINDOW_START
require_env REPORT_WINDOW_END
require_env REPORT_POLICY_LIVE_SMOKE_OUTPUT

if ((${#missing[@]} > 0)); then
  printf '[report-policy-live-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

if [[ -z "${OPENCLARION_LLM_MODEL:-}" ]]; then
  if [[ "${REPORT_LIVE_SMOKE_ASSUME_WORKER_READY:-}" != "1" ]]; then
    cat >&2 <<'EOF'
[report-policy-live-smoke] cannot confirm worker LLM configuration.
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
[report-policy-live-smoke] cannot confirm worker notification configuration.
Set OPENCLARION_IM_WEBHOOK_URL for legacy/unbound report delivery, set
OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON for profile-bound report
delivery, or set REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1 after confirming a
report-capable worker is already running with the required notification config.
EOF
    exit 2
  fi
fi

limit="${REPORT_REPLAY_LIMIT:-10000}"
wait_timeout="${REPORT_WAIT_TIMEOUT:-20m}"
output="$REPORT_POLICY_LIVE_SMOKE_OUTPUT"

args=(
  report-policy-replay
  --policy-id "$REPORT_WORKFLOW_POLICY_ID"
  --window-start "$REPORT_WINDOW_START"
  --window-end "$REPORT_WINDOW_END"
  --limit "$limit"
  --wait
  --wait-timeout "$wait_timeout"
)

if [[ -n "${REPORT_CORRELATION_KEY:-}" ]]; then
  args+=(--correlation-key "$REPORT_CORRELATION_KEY")
fi
if [[ -n "${REPORT_WORKFLOW_ID:-}" ]]; then
  args+=(--workflow-id "$REPORT_WORKFLOW_ID")
fi

mkdir -p "$(dirname "$output")"

echo "[report-policy-live-smoke] running policy-driven report replay and waiting for workflow completion..." >&2
go run ./scripts/manual_evidence_readiness --target report-policy-live-smoke --require-ready >/dev/null
go run ./cmd/openclarion "${args[@]}" >"$output"

go run ./scripts/report_live_smoke_output "$output"

echo "[report-policy-live-smoke] OK - live smoke output: $output" >&2
