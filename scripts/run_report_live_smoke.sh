#!/usr/bin/env bash
# Run the manual M2 live report smoke check against real external services.
#
# This is intentionally NOT part of make ci: it requires a live Prometheus,
# PostgreSQL, Temporal, and a running OpenClarion worker with LLM + IM
# providers configured.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

private_output_dir="${REPORT_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/report-live-smoke}"
mkdir -p "$private_output_dir"
private_output_dir="$(cd "$private_output_dir" && pwd -P)"
chmod 700 "$private_output_dir"

private_output_file() {
  local name="$1"
  mktemp "$private_output_dir/${name}.XXXXXX.json"
}

env_truthy() {
  case "${1:-}" in
    1 | true | TRUE | yes | YES)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

require_env DATABASE_URL
require_env TEMPORAL_HOST_PORT
require_env OPENCLARION_PROMETHEUS_URL
require_env REPORT_WINDOW_START
require_env REPORT_WINDOW_END

if ((${#missing[@]} > 0)); then
  printf '[report-live-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

if [[ -z "${OPENCLARION_LLM_MODEL:-}" || -z "${OPENCLARION_IM_WEBHOOK_URL:-}" ]]; then
  if [[ "${REPORT_LIVE_SMOKE_ASSUME_WORKER_READY:-}" != "1" ]]; then
    cat >&2 <<'EOF'
[report-live-smoke] cannot confirm worker provider configuration.
Set OPENCLARION_LLM_MODEL and OPENCLARION_IM_WEBHOOK_URL in this shell, or set
REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1 after confirming a report-capable worker
is already running with OPENCLARION_LLM_* and OPENCLARION_IM_WEBHOOK_* config.
EOF
    exit 2
  fi
fi

scenario="${REPORT_SCENARIO:-single_alert}"
limit="${REPORT_REPLAY_LIMIT:-10000}"
wait_timeout="${REPORT_WAIT_TIMEOUT:-20m}"
output="${REPORT_LIVE_SMOKE_OUTPUT:-$(private_output_file output)}"

args=(
  report-replay
  --window-start "$REPORT_WINDOW_START"
  --window-end "$REPORT_WINDOW_END"
  --limit "$limit"
  --scenario "$scenario"
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

echo "[report-live-smoke] running report replay and waiting for workflow completion..." >&2
go run ./cmd/openclarion "${args[@]}" >"$output"

if env_truthy "${REPORT_LIVE_SMOKE_REQUIRE_AI_REVIEW:-}"; then
  ai_review_wait_timeout="${REPORT_LIVE_SMOKE_AI_REVIEW_WAIT_TIMEOUT:-10m}"
  ai_review_poll_interval="${REPORT_LIVE_SMOKE_AI_REVIEW_POLL_INTERVAL:-5s}"
  echo "[report-live-smoke] enriching report proof with diagnosis-room AI review evidence..." >&2
  go run ./scripts/report_ai_review_proof \
    --input "$output" \
    --output "$output" \
    --wait-timeout "$ai_review_wait_timeout" \
    --poll-interval "$ai_review_poll_interval"
  go run ./scripts/report_live_smoke_output --require-ai-review "$output"
else
  go run ./scripts/report_live_smoke_output "$output"
fi

echo "[report-live-smoke] OK - live smoke output: $output" >&2
