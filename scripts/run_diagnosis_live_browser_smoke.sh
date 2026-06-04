#!/usr/bin/env bash
# Run the manual M5 live browser smoke check against a real OpenClarion
# backend/worker stack.
#
# This is intentionally NOT part of make ci: it requires a running backend,
# a real AuthProvider token, and a sandbox-capable worker. It can either use
# an existing DiagnosisRoomWorkflow session or create one from a frozen
# EvidenceSnapshot before launching the browser check.

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

require_env OPENCLARION_LIVE_API_BASE_URL
require_env OPENCLARION_LIVE_BEARER_TOKEN
if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID:-}" && -z "${OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID:-}" ]]; then
  missing+=("OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID or OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID")
fi
close_notification_required="${OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION:-${DIAGNOSIS_LIVE_REQUIRE_CLOSE_NOTIFICATION:-}}"
if env_truthy "$close_notification_required"; then
  require_env DATABASE_URL
fi

if ((${#missing[@]} > 0)); then
  printf '[diagnosis-live-browser-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

output="${DIAGNOSIS_LIVE_BROWSER_SMOKE_OUTPUT:-$(mktemp -t openclarion-diagnosis-live-browser-smoke.XXXXXX.json)}"
mkdir -p "$(dirname "$output")"
browser_proof="$(mktemp -t openclarion-diagnosis-live-browser-proof.XXXXXX.json)"
export OPENCLARION_LIVE_BROWSER_PROOF_PATH="$browser_proof"

if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_MESSAGE:-}" ]]; then
  export OPENCLARION_LIVE_DIAGNOSIS_MESSAGE="Live browser acceptance $(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi

normalize_bearer_env() {
  local raw trimmed token
  raw="${OPENCLARION_LIVE_BEARER_TOKEN:-}"
  trimmed="$(printf '%s' "$raw" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if [[ -z "$trimmed" ]]; then
    echo "[diagnosis-live-browser-smoke] OPENCLARION_LIVE_BEARER_TOKEN must be non-empty" >&2
    exit 2
  fi
  if [[ "$trimmed" =~ ^[Bb][Ee][Aa][Rr][Ee][Rr][[:space:]]+(.+)$ ]]; then
    token="${BASH_REMATCH[1]}"
  else
    token="$trimmed"
  fi
  if [[ -z "$token" || "$token" =~ [[:space:]] ]]; then
    echo "[diagnosis-live-browser-smoke] OPENCLARION_LIVE_BEARER_TOKEN must be a single bearer token or Bearer header" >&2
    exit 2
  fi
  export OPENCLARION_LIVE_BEARER_TOKEN="$token"
  export OPENCLARION_LIVE_AUTHORIZATION_HEADER="Bearer $token"
}

normalize_bearer_env

if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID:-}" ]]; then
  echo "[diagnosis-live-browser-smoke] creating live diagnosis room from evidence snapshot ${OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID}..." >&2
  create_response="$(
    node <<'EOF'
const baseURL = process.env.OPENCLARION_LIVE_API_BASE_URL;
const authorizationHeader = process.env.OPENCLARION_LIVE_AUTHORIZATION_HEADER;
const evidenceSnapshotID = Number(process.env.OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID);

if (!Number.isSafeInteger(evidenceSnapshotID) || evidenceSnapshotID <= 0) {
  throw new Error("OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID must be a positive integer");
}
if (!authorizationHeader || !/^Bearer [^ \t\r\n]+$/.test(authorizationHeader)) {
  throw new Error("OPENCLARION_LIVE_AUTHORIZATION_HEADER must be a Bearer header");
}

(async () => {
  const response = await fetch(new URL("/api/v1/diagnosis/rooms", baseURL), {
    method: "POST",
    headers: {
      "authorization": authorizationHeader,
      "content-type": "application/json",
      "accept": "application/json"
    },
    body: JSON.stringify({ evidence_snapshot_id: evidenceSnapshotID })
  });
  const bodyText = await response.text();
  if (!response.ok) {
    throw new Error(`create diagnosis room failed with HTTP ${response.status}: ${bodyText}`);
  }
  const body = JSON.parse(bodyText);
  if (!body.session_id) {
    throw new Error(`create diagnosis room response missing session_id: ${bodyText}`);
  }
  process.stdout.write(JSON.stringify(body));
})().catch((error) => {
  console.error(error.message || error);
  process.exit(1);
});
EOF
  )"
  export OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE="$create_response"
  export OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID="$(
    node -e 'const body = JSON.parse(process.env.OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE); process.stdout.write(body.session_id);'
  )"
fi

echo "[diagnosis-live-browser-smoke] installing frontend dependencies..." >&2
(cd web && npm ci)

if [[ -z "${OPENCLARION_LIVE_WEB_BASE_URL:-}" ]]; then
  echo "[diagnosis-live-browser-smoke] building local production Next.js server..." >&2
  (cd web && npm run build)
fi

echo "[diagnosis-live-browser-smoke] installing Chromium browser dependencies..." >&2
(cd web && npx playwright install --with-deps chromium)

echo "[diagnosis-live-browser-smoke] running browser round trip against live backend..." >&2
(cd web && npm run smoke:live)

if env_truthy "$close_notification_required"; then
  close_output="$(mktemp -t openclarion-diagnosis-live-close.XXXXXX.json)"
  close_reason="${OPENCLARION_LIVE_CLOSE_REASON:-live_smoke_completed}"
  close_wait_timeout="${OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT:-2m}"
  close_args=(
    diagnosis-room-close
    --session-id "$OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID"
    --reason "$close_reason"
    --wait-timeout "$close_wait_timeout"
  )
  close_run_id="${OPENCLARION_LIVE_DIAGNOSIS_RUN_ID:-}"
  if [[ -n "${OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE:-}" ]]; then
    close_run_id="$(
      node -e 'const body = JSON.parse(process.env.OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE); if (body.run_id) process.stdout.write(body.run_id);'
    )"
  fi
  if [[ -n "$close_run_id" ]]; then
    close_args+=(--run-id "$close_run_id")
  fi
  echo "[diagnosis-live-browser-smoke] closing diagnosis room and validating close notification..." >&2
  go run ./cmd/openclarion "${close_args[@]}" >"$close_output"
  export OPENCLARION_LIVE_CLOSE_NOTIFICATION_PROOF_PATH="$close_output"
fi

node - "$output" <<'EOF'
const { createHash } = require("node:crypto");
const fs = require("node:fs");

function sha256Hex(value) {
  return createHash("sha256").update(value, "utf8").digest("hex");
}

function canonicalUTCNow() {
  return new Date().toISOString().replace(/\.000Z$/, "Z").replace(/(\.\d*?[1-9])0+Z$/, "$1Z");
}

const output = process.argv[2];
const browserProofPath = process.env.OPENCLARION_LIVE_BROWSER_PROOF_PATH;
if (!browserProofPath) {
  throw new Error("OPENCLARION_LIVE_BROWSER_PROOF_PATH is required");
}
const browser = JSON.parse(fs.readFileSync(browserProofPath, "utf8"));
const createdRoom = process.env.OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE
  ? JSON.parse(process.env.OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE)
  : null;
const closeNotification = process.env.OPENCLARION_LIVE_CLOSE_NOTIFICATION_PROOF_PATH
  ? JSON.parse(fs.readFileSync(process.env.OPENCLARION_LIVE_CLOSE_NOTIFICATION_PROOF_PATH, "utf8"))
  : null;
const evidenceSnapshotID = process.env.OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID
  ? Number(process.env.OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID)
  : null;
if (evidenceSnapshotID !== null && (!Number.isSafeInteger(evidenceSnapshotID) || evidenceSnapshotID <= 0)) {
  throw new Error("OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID must be a positive integer");
}
const message = (process.env.OPENCLARION_LIVE_DIAGNOSIS_MESSAGE || "").trim();
const messageLength = message.length;
const messageSHA256 = sha256Hex(message);
const proof = {
  passed: true,
  checked_at: canonicalUTCNow(),
  request: {
    mode: createdRoom ? "create_room" : "existing_session",
    session_id: process.env.OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID,
    evidence_snapshot_id: evidenceSnapshotID,
    message_length: messageLength,
    message_sha256: messageSHA256
  },
  web_base_url: process.env.OPENCLARION_LIVE_WEB_BASE_URL || `http://127.0.0.1:${process.env.OPENCLARION_LIVE_WEB_PORT || "32101"}`,
  api_base_url: process.env.OPENCLARION_LIVE_API_BASE_URL,
  session_id: process.env.OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID,
  evidence_snapshot_id: evidenceSnapshotID,
  created_room: createdRoom,
  message_length: messageLength,
  message_sha256: messageSHA256,
  browser,
  close_notification: closeNotification,
  evidence: closeNotification
    ? "Playwright live diagnosis-room browser smoke passed one connect/query/submit/turn_result round trip and close_notification proof."
    : "Playwright live diagnosis-room browser smoke passed one connect/query/submit/turn_result round trip."
};

fs.writeFileSync(output, `${JSON.stringify(proof, null, 2)}\n`);
EOF

go run ./scripts/diagnosis_live_smoke_output "$output"

echo "[diagnosis-live-browser-smoke] OK - live smoke output: $output" >&2
