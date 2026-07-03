#!/usr/bin/env bash
# Run the manual M5 live browser smoke check against a real OpenClarion
# backend/worker stack.
#
# This is intentionally NOT part of make ci: it requires a running backend,
# a real AuthProvider token, and a sandbox-capable worker. It can either use
# an existing DiagnosisRoomWorkflow session or create one from a frozen
# EvidenceSnapshot before launching the browser check.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

private_tmp_dir="${OPENCLARION_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/diagnosis-live-browser-smoke}"
mkdir -p "$private_tmp_dir"
private_tmp_dir="$(cd "$private_tmp_dir" && pwd -P)"
chmod 700 "$private_tmp_dir"

private_temp_file() {
  local name="$1"
  mktemp "$private_tmp_dir/${name}.XXXXXX.json"
}

missing=()
require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    missing+=("$key")
  fi
}

fail() {
  echo "[diagnosis-live-browser-smoke] $1" >&2
  exit 2
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

positive_integer() {
  [[ "${1:-}" =~ ^[1-9][0-9]*$ ]]
}

detect_live_auth_mode() {
  local diagnosis_auth_mode
  if [[ -z "${OPENCLARION_LIVE_AUTH_MODE:-}" ]]; then
    if [[ -n "${OPENCLARION_LIVE_LDAP_USERNAME:-}${OPENCLARION_LIVE_LDAP_PASSWORD:-}" ]]; then
      OPENCLARION_LIVE_AUTH_MODE="ldap"
    else
      diagnosis_auth_mode="${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}"
      diagnosis_auth_mode="${diagnosis_auth_mode,,}"
      if [[ "$diagnosis_auth_mode" == "ldap" ]]; then
        OPENCLARION_LIVE_AUTH_MODE="ldap"
      else
        OPENCLARION_LIVE_AUTH_MODE="bearer"
      fi
    fi
  fi
  OPENCLARION_LIVE_AUTH_MODE="${OPENCLARION_LIVE_AUTH_MODE,,}"
  case "$OPENCLARION_LIVE_AUTH_MODE" in
    ldap | bearer)
      export OPENCLARION_LIVE_AUTH_MODE
      ;;
    *)
      fail "OPENCLARION_LIVE_AUTH_MODE must be ldap or bearer"
      ;;
  esac
}

json_array_length_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    printf '0'
    return
  fi
  KEY="$key" node <<'EOF'
const key = process.env.KEY;
const raw = process.env[key];
try {
  const decoded = JSON.parse(raw);
  if (!Array.isArray(decoded)) {
    throw new Error("array");
  }
  process.stdout.write(String(decoded.length));
} catch {
  console.error(`[diagnosis-live-browser-smoke] ${key} must be a JSON array`);
  process.exit(2);
}
EOF
}

default_live_turn_timeout_ms="${OPENCLARION_LIVE_DEFAULT_TURN_TIMEOUT_MS:-300000}"
if ! positive_integer "$default_live_turn_timeout_ms"; then
  echo "[diagnosis-live-browser-smoke] OPENCLARION_LIVE_DEFAULT_TURN_TIMEOUT_MS must be a positive integer" >&2
  exit 2
fi

require_env OPENCLARION_LIVE_API_BASE_URL
detect_live_auth_mode
case "$OPENCLARION_LIVE_AUTH_MODE" in
  ldap)
    require_env OPENCLARION_LIVE_LDAP_USERNAME
    require_env OPENCLARION_LIVE_LDAP_PASSWORD
    ;;
  bearer)
    if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN:-}" &&
          -z "${OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN:-}" &&
          -z "${OPENCLARION_LIVE_BEARER_TOKEN:-}" &&
          -z "${OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL:-}" ]]; then
      missing+=("OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN, OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN, OPENCLARION_LIVE_BEARER_TOKEN, OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL, or OPENCLARION_LIVE_LDAP_USERNAME + OPENCLARION_LIVE_LDAP_PASSWORD")
    fi
    ;;
esac
if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID:-}" && -z "${OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID:-}" ]]; then
  missing+=("OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID or OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID")
fi
close_notification_required="${OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION:-${DIAGNOSIS_LIVE_REQUIRE_CLOSE_NOTIFICATION:-}}"
if [[ -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}" ]]; then
  if ! positive_integer "$OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID"; then
    echo "[diagnosis-live-browser-smoke] OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID must be a positive integer when set" >&2
    exit 2
  fi
  close_notification_required="1"
fi
if env_truthy "$close_notification_required"; then
  require_env DATABASE_URL
  require_env TEMPORAL_HOST_PORT
fi

if ((${#missing[@]} > 0)); then
  printf '[diagnosis-live-browser-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

if [[ -z "${OPENCLARION_LIVE_TURN_TIMEOUT_MS:-}" ]]; then
  if [[ -n "${OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS:-}" ]]; then
    if ! positive_integer "$OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS"; then
      echo "[diagnosis-live-browser-smoke] OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS must be a positive integer when deriving live timeout" >&2
      exit 2
    fi
    derived_live_turn_timeout_ms="$(((OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS * 2 + 120) * 1000))"
    if ((derived_live_turn_timeout_ms > default_live_turn_timeout_ms)); then
      export OPENCLARION_LIVE_TURN_TIMEOUT_MS="$derived_live_turn_timeout_ms"
    else
      export OPENCLARION_LIVE_TURN_TIMEOUT_MS="$default_live_turn_timeout_ms"
    fi
  else
    export OPENCLARION_LIVE_TURN_TIMEOUT_MS="$default_live_turn_timeout_ms"
  fi
elif ! positive_integer "$OPENCLARION_LIVE_TURN_TIMEOUT_MS"; then
  echo "[diagnosis-live-browser-smoke] OPENCLARION_LIVE_TURN_TIMEOUT_MS must be a positive integer" >&2
  exit 2
fi

if [[ -z "${OPENCLARION_LIVE_TEST_TIMEOUT_MS:-}" ]]; then
  expected_live_turns=1
  expected_live_turns=$((expected_live_turns + $(json_array_length_env OPENCLARION_LIVE_OPERATOR_TOOL_REQUESTS_JSON)))
  if env_truthy "${OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE:-}"; then
    expected_live_turns=$((expected_live_turns + 1))
  fi
  if env_truthy "${OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE:-}"; then
    expected_live_turns=$((expected_live_turns + 1))
  fi
  export OPENCLARION_LIVE_TEST_TIMEOUT_MS="$((OPENCLARION_LIVE_TURN_TIMEOUT_MS * expected_live_turns + 60000))"
elif ! positive_integer "$OPENCLARION_LIVE_TEST_TIMEOUT_MS"; then
  echo "[diagnosis-live-browser-smoke] OPENCLARION_LIVE_TEST_TIMEOUT_MS must be a positive integer" >&2
  exit 2
fi

output="${DIAGNOSIS_LIVE_BROWSER_SMOKE_OUTPUT:-$(private_temp_file output)}"
mkdir -p "$(dirname "$output")"
browser_proof="$(private_temp_file browser-proof)"
export OPENCLARION_LIVE_BROWSER_PROOF_PATH="$browser_proof"

readiness_output="$(private_temp_file readiness)"
if ! go run ./scripts/manual_evidence_readiness --target diagnosis-live-browser-smoke --require-ready >"$readiness_output"; then
  echo "[diagnosis-live-browser-smoke] readiness preflight failed; details: $readiness_output" >&2
  exit 2
fi

refresh_dev_oidc_token() {
  if [[ -z "${OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL:-}" ]]; then
    return
  fi
  echo "[diagnosis-live-browser-smoke] fetching fresh local dev OIDC bearer token..." >&2
  OPENCLARION_LIVE_BEARER_TOKEN="$(
    node <<'EOF'
const tokenURL = process.env.OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL;
if (!tokenURL) {
  process.exit(0);
}

function fail(message) {
  console.error(`[diagnosis-live-browser-smoke] ${message}`);
  process.exit(2);
}

function isLoopbackHost(hostname) {
  const host = hostname.replace(/^\[|\]$/g, "").toLowerCase();
  return host === "localhost" || host === "::1" || /^127(?:\.\d{1,3}){3}$/.test(host);
}

let url;
try {
  url = new URL(tokenURL);
} catch {
  fail("OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL must be a valid URL");
}
if (!["http:", "https:"].includes(url.protocol)) {
  fail("OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL must use http or https");
}
if (!isLoopbackHost(url.hostname)) {
  fail("OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL must use a loopback host");
}
if (url.username || url.password || url.hash) {
  fail("OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL must not include userinfo or fragment");
}
for (const [envName, queryName] of [
  ["OPENCLARION_LIVE_DEV_OIDC_SUBJECT", "subject"],
  ["OPENCLARION_LIVE_DEV_OIDC_ROLES", "roles"],
  ["OPENCLARION_LIVE_DEV_OIDC_AUDIENCE", "audience"],
  ["OPENCLARION_LIVE_DEV_OIDC_TTL", "ttl"],
]) {
  const value = process.env[envName];
  if (value) {
    url.searchParams.set(queryName, value);
  }
}

(async () => {
  const response = await fetch(url, { headers: { accept: "application/json" } });
  const bodyText = await response.text();
  if (!response.ok) {
    fail(`dev OIDC token endpoint returned HTTP ${response.status}`);
  }
  let body;
  try {
    body = JSON.parse(bodyText);
  } catch {
    fail("dev OIDC token endpoint returned invalid JSON");
  }
  const raw = String(body.id_token || body.authorization_header || "").trim();
  const match = raw.match(/^Bearer\s+(.+)$/i);
  const token = match ? match[1] : raw;
  if (!token || /\s/.test(token)) {
    fail("dev OIDC token endpoint did not return a single bearer token");
  }
  process.stdout.write(token);
})().catch((error) => {
  fail(error?.message || "dev OIDC token fetch failed");
});
EOF
  )"
  export OPENCLARION_LIVE_BEARER_TOKEN
}

normalize_bearer_env() {
  local raw trimmed token diagnosis_auth_mode
  raw="${OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN:-}"
  if [[ -z "$raw" ]]; then
    diagnosis_auth_mode="${OPENCLARION_DIAGNOSIS_AUTH_MODE:-}"
    diagnosis_auth_mode="${diagnosis_auth_mode,,}"
    if [[ -n "${OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN:-}" &&
          ( "$diagnosis_auth_mode" == "static" || -z "$diagnosis_auth_mode" ) ]]; then
      raw="$OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN"
    fi
  fi
  if [[ -z "$raw" ]]; then
    raw="${OPENCLARION_LIVE_BEARER_TOKEN:-}"
  fi
  trimmed="$(printf '%s' "$raw" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if [[ -z "$trimmed" ]]; then
    echo "[diagnosis-live-browser-smoke] bearer token must be non-empty" >&2
    exit 2
  fi
  if [[ "$trimmed" =~ ^[Bb][Ee][Aa][Rr][Ee][Rr][[:space:]]+(.+)$ ]]; then
    token="${BASH_REMATCH[1]}"
  else
    token="$trimmed"
  fi
  if [[ -z "$token" || "$token" =~ [[:space:]] ]]; then
    echo "[diagnosis-live-browser-smoke] bearer token must be a single bearer token or Bearer header" >&2
    exit 2
  fi
  export OPENCLARION_LIVE_BEARER_TOKEN="$token"
  export OPENCLARION_LIVE_AUTHORIZATION_HEADER="Bearer $token"
}

normalize_ldap_env() {
  if [[ -z "${OPENCLARION_LIVE_LDAP_USERNAME:-}" || -z "${OPENCLARION_LIVE_LDAP_PASSWORD:-}" ]]; then
    fail "OPENCLARION_LIVE_LDAP_USERNAME and OPENCLARION_LIVE_LDAP_PASSWORD are required for ldap auth mode"
  fi
  if [[ "${OPENCLARION_LIVE_LDAP_USERNAME}" =~ [[:space:]] ]]; then
    fail "OPENCLARION_LIVE_LDAP_USERNAME must not contain whitespace"
  fi
  if [[ "$OPENCLARION_LIVE_LDAP_PASSWORD" == *$'\n'* || "$OPENCLARION_LIVE_LDAP_PASSWORD" == *$'\r'* ]]; then
    fail "OPENCLARION_LIVE_LDAP_PASSWORD must not contain CR or LF"
  fi
  OPENCLARION_LIVE_AUTHORIZATION_HEADER="$(
    node <<'EOF'
const username = process.env.OPENCLARION_LIVE_LDAP_USERNAME || "";
const password = process.env.OPENCLARION_LIVE_LDAP_PASSWORD || "";
if (!username || !password) {
  console.error("[diagnosis-live-browser-smoke] LDAP credentials are required");
  process.exit(2);
}
process.stdout.write(`Basic ${Buffer.from(`${username}:${password}`, "utf8").toString("base64")}`);
EOF
  )"
  export OPENCLARION_LIVE_AUTHORIZATION_HEADER
}

prepare_live_auth() {
  detect_live_auth_mode
  case "$OPENCLARION_LIVE_AUTH_MODE" in
    ldap)
      normalize_ldap_env
      ;;
    bearer)
      refresh_dev_oidc_token
      normalize_bearer_env
      ;;
  esac
}

prepare_live_auth

if [[ -z "${OPENCLARION_LIVE_BROWSER_WS_BASE_URL:-}" ]]; then
  export OPENCLARION_LIVE_BROWSER_WS_BASE_URL="$OPENCLARION_LIVE_API_BASE_URL"
fi
if [[ -z "${OPENCLARION_BROWSER_WS_BASE_URL:-}" ]]; then
  export OPENCLARION_BROWSER_WS_BASE_URL="$OPENCLARION_LIVE_BROWSER_WS_BASE_URL"
fi

node <<'EOF'
const raw = process.env.OPENCLARION_BROWSER_WS_BASE_URL;
try {
  const url = new URL(raw);
  if (!["http:", "https:", "ws:", "wss:"].includes(url.protocol)) {
    throw new Error("scheme");
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new Error("credential-or-state");
  }
} catch {
  console.error("[diagnosis-live-browser-smoke] OPENCLARION_BROWSER_WS_BASE_URL must be an http(s) or ws(s) base URL without userinfo, query, or fragment");
  process.exit(2);
}
EOF

if [[ -z "${OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID:-}" ]]; then
  echo "[diagnosis-live-browser-smoke] creating live diagnosis room from evidence snapshot ${OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID}..." >&2
  create_response="$(
    node <<'EOF'
const baseURL = process.env.OPENCLARION_LIVE_API_BASE_URL;
const authorizationHeader = process.env.OPENCLARION_LIVE_AUTHORIZATION_HEADER;
const evidenceSnapshotID = Number(process.env.OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID);
const notificationChannelProfileID = process.env.OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID
  ? Number(process.env.OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID)
  : 0;

if (!Number.isSafeInteger(evidenceSnapshotID) || evidenceSnapshotID <= 0) {
  throw new Error("OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID must be a positive integer");
}
if (notificationChannelProfileID !== 0 && (!Number.isSafeInteger(notificationChannelProfileID) || notificationChannelProfileID <= 0)) {
  throw new Error("OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID must be a positive integer");
}
if (!authorizationHeader || !/^(Bearer|Basic) [^ \t\r\n]+$/i.test(authorizationHeader)) {
  throw new Error("OPENCLARION_LIVE_AUTHORIZATION_HEADER must be a Bearer or Basic header");
}

(async () => {
  const requestBody = { evidence_snapshot_id: evidenceSnapshotID };
  if (notificationChannelProfileID > 0) {
    requestBody.close_notification_channel_profile_id = notificationChannelProfileID;
  }
  const response = await fetch(new URL("/api/v1/diagnosis/rooms", baseURL), {
    method: "POST",
    headers: {
      "authorization": authorizationHeader,
      "content-type": "application/json",
      "accept": "application/json"
    },
    body: JSON.stringify(requestBody)
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
  created_session_id="$(
    node -e 'const body = JSON.parse(process.env.OPENCLARION_LIVE_DIAGNOSIS_ROOM_CREATE_RESPONSE); process.stdout.write(body.session_id);'
  )"
  export OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID="$created_session_id"
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
(cd web && npm run smoke:live:built)

if env_truthy "$close_notification_required"; then
  close_output="$(private_temp_file close-proof)"
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

if [[ -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}" ]]; then
  notification_proof="$(private_temp_file notification-proof)"
  export OPENCLARION_LIVE_NOTIFICATION_PROOF_PATH="$notification_proof"
  echo "[diagnosis-live-browser-smoke] validating retained Enterprise WeChat AI notification proof..." >&2
  node <<'EOF'
const fs = require("node:fs");

const apiBase = requiredEnv("OPENCLARION_LIVE_API_BASE_URL").replace(/\/+$/, "");
const authorizationHeader = requiredEnv("OPENCLARION_LIVE_AUTHORIZATION_HEADER");
const outputPath = requiredEnv("OPENCLARION_LIVE_NOTIFICATION_PROOF_PATH");
const sessionID = requiredEnv("OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID");
const notificationChannelProfileID = positiveIntegerEnv("OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID");
const timeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_NOTIFICATION_PROOF_TIMEOUT_MS", 60000);
const pollMS = positiveIntegerEnv("OPENCLARION_LIVE_NOTIFICATION_PROOF_POLL_MS", 5000);
const requiredEventKinds = [
  "diagnosis_room.assistant_turn_notification_sent",
  "diagnosis_room.final_ready_notification_sent",
  "diagnosis_room.close_notification_sent"
];
const acceptedProviderStatuses = new Set(["accepted", "delivered", "sent", "success"]);

function requiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function positiveIntegerEnv(name, fallback) {
  const value = process.env[name]?.trim();
  if (!value) {
    if (fallback === undefined) {
      throw new Error(`${name} is required`);
    }
    return fallback;
  }
  if (!/^[1-9][0-9]*$/.test(value)) {
    throw new Error(`${name} must be a positive integer`);
  }
  return Number(value);
}

function canonicalUTCNow() {
  return new Date().toISOString().replace(/\.000Z$/, "Z").replace(/(\.\d*?[1-9])0+Z$/, "$1Z");
}

function canonicalUTC(value) {
  const raw = String(value || "").trim();
  if (!raw) {
    return "";
  }
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toISOString().replace(/\.000Z$/, "Z").replace(/(\.\d*?[1-9])0+Z$/, "$1Z");
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function apiJSON(pathname) {
  const response = await fetch(new URL(pathname, apiBase), {
    headers: {
      authorization: authorizationHeader,
      accept: "application/json"
    }
  });
  if (!response.ok) {
    throw new Error(`${pathname} returned HTTP ${response.status}`);
  }
  return await response.json();
}

function expectedContentKind(eventKind) {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return "assistant_message";
    case "diagnosis_room.final_ready_notification_sent":
    case "diagnosis_room.close_notification_sent":
      return "final_conclusion";
    default:
      return "";
  }
}

function acceptedAIEntry(entry) {
  if (!entry || typeof entry !== "object" || !requiredEventKinds.includes(entry.event_kind)) {
    return false;
  }
  if (entry.notification_channel_profile_id !== notificationChannelProfileID) {
    return false;
  }
  const status = String(entry.provider_status || "").trim().toLowerCase();
  if (!acceptedProviderStatuses.has(status)) {
    return false;
  }
  if (entry.content_kind !== expectedContentKind(entry.event_kind)) {
    return false;
  }
  return /^[a-f0-9]{64}$/.test(String(entry.content_sha256 || ""));
}

function proofEntry(entry) {
  return {
    event_kind: entry.event_kind,
    notification_channel_profile_id: entry.notification_channel_profile_id || null,
    provider_status: entry.provider_status || "",
    provider_message_id: entry.provider_message_id || "",
    assistant_message_id: entry.assistant_message_id || "",
    assistant_turn_id: entry.assistant_turn_id || null,
    turn_count: entry.turn_count || null,
    content_kind: entry.content_kind || "",
    content_sha256: entry.content_sha256 || "",
    recommended_action_count: entry.recommended_action_count ?? null,
    evidence_request_count: entry.evidence_request_count ?? null,
    occurred_at: canonicalUTC(entry.occurred_at)
  };
}

function hasRequiredEvents(entries) {
  const seen = new Set(entries.map((entry) => entry.event_kind));
  return requiredEventKinds.every((eventKind) => seen.has(eventKind));
}

function writeProof(proof) {
  fs.writeFileSync(outputPath, `${JSON.stringify(proof, null, 2)}\n`, { mode: 0o600 });
}

(async () => {
  const started = Date.now();
  let lastTimeline = [];
  while (Date.now() - started <= timeoutMS) {
    const summary = await apiJSON(`/api/v1/diagnosis/rooms/${encodeURIComponent(sessionID)}`);
    const timeline = Array.isArray(summary.notification_timeline) ? summary.notification_timeline : [];
    lastTimeline = timeline;
    const entries = timeline.filter(acceptedAIEntry).map(proofEntry);
    if (hasRequiredEvents(entries)) {
      writeProof({
        checked_at: canonicalUTCNow(),
        requested: true,
        passed: true,
        entries
      });
      return;
    }
    await sleep(pollMS);
  }
  writeProof({
    checked_at: canonicalUTCNow(),
    requested: true,
    passed: false,
    skipped_reason: "ai_notification_content_proof_not_found",
    entries: lastTimeline.map(proofEntry)
  });
  throw new Error("retained AI notification content proof did not cover assistant update, final conclusion, and close notification");
})().catch((error) => {
  console.error(`[diagnosis-live-browser-smoke] ${error?.message || error}`);
  process.exit(1);
});
EOF
fi

node - "$output" <<'EOF'
const fs = require("node:fs");

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
const notificationProof = process.env.OPENCLARION_LIVE_NOTIFICATION_PROOF_PATH
  ? JSON.parse(fs.readFileSync(process.env.OPENCLARION_LIVE_NOTIFICATION_PROOF_PATH, "utf8"))
  : null;
const evidenceSnapshotID = process.env.OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID
  ? Number(process.env.OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID)
  : null;
if (evidenceSnapshotID !== null && (!Number.isSafeInteger(evidenceSnapshotID) || evidenceSnapshotID <= 0)) {
  throw new Error("OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID must be a positive integer");
}
const notificationChannelProfileID = process.env.OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID
  ? Number(process.env.OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID)
  : null;
if (
  notificationChannelProfileID !== null &&
  (!Number.isSafeInteger(notificationChannelProfileID) || notificationChannelProfileID <= 0)
) {
  throw new Error("OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID must be a positive integer");
}
const messageLength = Number(browser.submitted_message_length);
const messageSHA256 = String(browser.submitted_message_sha256 || "");
if (!Number.isSafeInteger(messageLength) || messageLength <= 0) {
  throw new Error("browser.submitted_message_length must be a positive integer");
}
if (!/^[0-9a-f]{64}$/.test(messageSHA256)) {
  throw new Error("browser.submitted_message_sha256 must be a lowercase sha256 hex digest");
}
const browserConfirmedConclusion = browser.confirm_conclusion_requested === true &&
  browser.final_conclusion_confirmed === true;
const browserSatisfiedPlannedEvidence = browser.planned_evidence_collection_requested === true &&
  browser.planned_evidence_collection_satisfied === true;
const browserSubmittedSupplementalEvidence = browser.supplemental_evidence_requested === true &&
  browser.supplemental_evidence_submitted === true;
const evidenceClaims = ["turn_result"];
if (browserSatisfiedPlannedEvidence) {
  evidenceClaims.push("planned_evidence_collection");
}
if (browserSubmittedSupplementalEvidence) {
  evidenceClaims.push("supplemental_evidence");
}
if (closeNotification) {
  evidenceClaims.push("close_notification");
} else if (browserConfirmedConclusion) {
  evidenceClaims.push("confirm_conclusion");
}
if (notificationProof) {
  evidenceClaims.push("ai_notification_delivery");
}
const proof = {
  passed: true,
  checked_at: canonicalUTCNow(),
  request: {
    mode: createdRoom ? "create_room" : "existing_session",
    session_id: process.env.OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID,
    evidence_snapshot_id: evidenceSnapshotID,
    notification_channel_profile_id: notificationChannelProfileID,
    require_notification_proof: notificationChannelProfileID !== null,
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
  notification_proof: notificationProof,
  evidence: `Playwright live diagnosis-room browser smoke passed ${evidenceClaims.join(" ")} proof.`
};

fs.writeFileSync(output, `${JSON.stringify(proof, null, 2)}\n`);
EOF

go run ./scripts/diagnosis_live_smoke_output "$output"

echo "[diagnosis-live-browser-smoke] OK - live smoke output: $output" >&2
