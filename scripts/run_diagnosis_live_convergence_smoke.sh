#!/usr/bin/env bash
# Run a backend-only live diagnosis convergence smoke against a real
# OpenClarion backend/worker stack.
#
# This intentionally skips the browser and talks to the diagnosis WebSocket
# directly. It creates a room from OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID unless
# OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID is provided, then validates the core
# product loop: AI turn, planned evidence collection, residual operator evidence
# boundary, ready_for_review convergence, and optional human confirmation.

set -euo pipefail
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

# shellcheck source=scripts/lib_private_env.sh
source "$ROOT_DIR/scripts/lib_private_env.sh"

env_file="${OPENCLARION_DIAGNOSIS_LIVE_CONVERGENCE_ENV_FILE:-}"

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/run_diagnosis_live_convergence_smoke.sh [--env-file PATH]

Required env:
  OPENCLARION_LIVE_API_BASE_URL
  plus either:
    OPENCLARION_LIVE_BEARER_TOKEN
    or OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN
    or OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN for static diagnosis auth
    or OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL
    or OPENCLARION_LIVE_LDAP_USERNAME + OPENCLARION_LIVE_LDAP_PASSWORD
  plus either:
    OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID
    or OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID

Optional env:
  OPENCLARION_LIVE_AUTH_MODE=ldap|bearer
  OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID
    requires assistant update, final conclusion, and close notification proof
  OPENCLARION_LIVE_SMOKE_WORKDIR
  DIAGNOSIS_LIVE_CONVERGENCE_SMOKE_OUTPUT
  OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE
  OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE
  OPENCLARION_LIVE_CONFIRM_CONCLUSION

PATH must be a regular, non-symlink file owned by the current user, with no
group/other permissions, and outside the OpenClarion repository or under the
repo-local ignored .openclarion-private/ directory.
EOF
}

while (($# > 0)); do
  case "$1" in
    --env-file)
      if (($# < 2)); then
        usage
        exit 2
      fi
      env_file="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

openclarion_capture_exported_env_overrides
openclarion_load_private_env_file "diagnosis-live-convergence-smoke" "$ROOT_DIR" "$env_file" || exit $?
openclarion_restore_exported_env_overrides

private_tmp_dir="${OPENCLARION_LIVE_SMOKE_WORKDIR:-$ROOT_DIR/.openclarion-private/diagnosis-live-convergence-smoke}"
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
  echo "[diagnosis-live-convergence-smoke] $1" >&2
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

default_live_turn_timeout_ms="${OPENCLARION_LIVE_DEFAULT_TURN_TIMEOUT_MS:-360000}"
if ! positive_integer "$default_live_turn_timeout_ms"; then
  echo "[diagnosis-live-convergence-smoke] OPENCLARION_LIVE_DEFAULT_TURN_TIMEOUT_MS must be a positive integer" >&2
  exit 2
fi
if [[ -z "${OPENCLARION_LIVE_TURN_TIMEOUT_MS:-}" ]]; then
  export OPENCLARION_LIVE_TURN_TIMEOUT_MS="$default_live_turn_timeout_ms"
elif ! positive_integer "$OPENCLARION_LIVE_TURN_TIMEOUT_MS"; then
  echo "[diagnosis-live-convergence-smoke] OPENCLARION_LIVE_TURN_TIMEOUT_MS must be a positive integer" >&2
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
if [[ -n "${OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID:-}" ]] && ! positive_integer "$OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID"; then
  echo "[diagnosis-live-convergence-smoke] OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID must be a positive integer when set" >&2
  exit 2
fi
if [[ -n "${OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID:-}" ]] && ! positive_integer "$OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID"; then
  echo "[diagnosis-live-convergence-smoke] OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID must be a positive integer when set" >&2
  exit 2
fi

if ((${#missing[@]} > 0)); then
  printf '[diagnosis-live-convergence-smoke] missing required env:' >&2
  printf ' %s' "${missing[@]}" >&2
  printf '\n' >&2
  exit 2
fi

readiness_output="$(private_temp_file readiness)"
if ! go run ./scripts/manual_evidence_readiness --target diagnosis-live-convergence-smoke --require-ready >"$readiness_output"; then
  echo "[diagnosis-live-convergence-smoke] readiness preflight failed; details: $readiness_output" >&2
  exit 2
fi

refresh_dev_oidc_token() {
  if [[ -z "${OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL:-}" ]]; then
    return
  fi
  echo "[diagnosis-live-convergence-smoke] fetching fresh local dev OIDC bearer token..." >&2
  OPENCLARION_LIVE_BEARER_TOKEN="$(
    node <<'EOF'
const tokenURL = process.env.OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL;

function fail(message) {
  console.error(`[diagnosis-live-convergence-smoke] ${message}`);
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
    echo "[diagnosis-live-convergence-smoke] bearer token must be non-empty" >&2
    exit 2
  fi
  if [[ "$trimmed" =~ ^[Bb][Ee][Aa][Rr][Ee][Rr][[:space:]]+(.+)$ ]]; then
    token="${BASH_REMATCH[1]}"
  else
    token="$trimmed"
  fi
  if [[ -z "$token" || "$token" =~ [[:space:]] ]]; then
    echo "[diagnosis-live-convergence-smoke] bearer token must be a single bearer token or Bearer header" >&2
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
  console.error("[diagnosis-live-convergence-smoke] LDAP credentials are required");
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

output="${DIAGNOSIS_LIVE_CONVERGENCE_SMOKE_OUTPUT:-$(private_temp_file convergence-proof)}"
mkdir -p "$(dirname "$output")"

node - "$output" <<'EOF'
const fs = require("node:fs");
const crypto = require("node:crypto");

const outputPath = process.argv[2];
const apiBase = requiredEnv("OPENCLARION_LIVE_API_BASE_URL").replace(/\/+$/, "");
const authorizationHeader = requiredEnv("OPENCLARION_LIVE_AUTHORIZATION_HEADER");
if (!/^(Bearer|Basic) [^ \t\r\n]+$/i.test(authorizationHeader)) {
  throw new Error("OPENCLARION_LIVE_AUTHORIZATION_HEADER must be a Bearer or Basic header");
}
const turnTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_TURN_TIMEOUT_MS", 360000);
const timeoutRecoveryMS = positiveIntegerEnv("OPENCLARION_LIVE_TIMEOUT_RECOVERY_MS", turnTimeoutMS);
const timeoutRecoveryPollMS = positiveIntegerEnv("OPENCLARION_LIVE_TIMEOUT_RECOVERY_POLL_MS", 5000);
const notificationProofTimeoutMS = positiveIntegerEnv("OPENCLARION_LIVE_NOTIFICATION_PROOF_TIMEOUT_MS", 60000);
const notificationProofPollMS = positiveIntegerEnv("OPENCLARION_LIVE_NOTIFICATION_PROOF_POLL_MS", 5000);
const existingSessionID = process.env.OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID?.trim() || "";
const evidenceSnapshotID = positiveIntegerEnv("OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID", 0);
const notificationChannelProfileID = positiveIntegerEnv("OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID", 0);
const collectPlannedEvidence = truthyEnvDefault("OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE", true);
const submitSupplementalEvidence = truthyEnvDefault("OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE", true);
const confirmConclusionEnv = truthyEnv("OPENCLARION_LIVE_CONFIRM_CONCLUSION");
const supplementalTextOverride = process.env.OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT?.trim() || "";
const supplementalTemplate = process.env.OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE?.trim() || "";
const initialMessage = process.env.OPENCLARION_LIVE_DIAGNOSIS_MESSAGE?.trim() || [
  "Live direct WebSocket convergence validation.",
  "Use the frozen alert context and available diagnosis tools to assess the current incident.",
  "Request executable evidence first when it can materially improve confidence.",
  "When executable evidence has been collected and only unavailable operator or DBA artifacts remain, produce a bounded ready_for_review conclusion with requires_human_review=true and explicit caveats instead of repeating the same request."
].join(" ");

const proof = {
  passed: false,
  checked_at: canonicalUTCNow(),
  mode: "direct_ws_convergence",
  request: {
    existing_session_id: existingSessionID || null,
    evidence_snapshot_id: evidenceSnapshotID || null,
    notification_channel_profile_id: notificationChannelProfileID || null,
    require_notification_proof: notificationChannelProfileID > 0,
    collect_planned_evidence: collectPlannedEvidence,
    submit_supplemental_evidence: submitSupplementalEvidence,
    confirm_conclusion_requested: confirmConclusionEnv
  },
  stages: []
};

function requiredEnv(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
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

function positiveIntegerEnv(name, fallback) {
  const raw = process.env[name]?.trim();
  if (!raw) {
    return fallback;
  }
  if (!/^[1-9][0-9]*$/.test(raw)) {
    throw new Error(`${name} must be a positive integer`);
  }
  return Number(raw);
}

function truthyEnv(name) {
  const value = process.env[name]?.trim().toLowerCase();
  return value === "1" || value === "true" || value === "yes";
}

function truthyEnvDefault(name, fallback) {
  const value = process.env[name]?.trim();
  if (!value) {
    return fallback;
  }
  return ["1", "true", "yes"].includes(value.toLowerCase());
}

function addStage(name, data = {}) {
  const stage = { at: canonicalUTCNow(), name, ...data };
  proof.stages.push(stage);
  console.error(`[diagnosis-live-convergence-smoke] ${name}: ${JSON.stringify(data)}`);
}

function writeProof() {
  proof.checked_at = canonicalUTCNow();
  fs.writeFileSync(outputPath, `${JSON.stringify(proof, null, 2)}\n`, { mode: 0o600 });
}

function sanitizeFrame(frame) {
  const insight = frame.consultation_insight || {};
  const missing = insight.missing_evidence_requests || frame.final_conclusion?.missing_evidence_requests || [];
  return {
    type: frame.type,
    code: frame.code,
    message: truncateText(frame.message || "", 512),
    status: frame.status,
    turn_count: frame.turn_count,
    in_flight: frame.in_flight,
    confidence: frame.confidence,
    requires_human_review: frame.requires_human_review,
    conclusion_status: insight.conclusion_status || frame.final_conclusion?.status || "",
    evidence_requests: Array.isArray(frame.evidence_requests) ? frame.evidence_requests.length : 0,
    collection_results: Array.isArray(frame.evidence_collection_results) ? frame.evidence_collection_results.length : 0,
    missing: Array.isArray(missing) ? missing.length : 0,
    final_conclusion_available: frame.final_conclusion?.status === "available",
    close_reason: frame.close_reason || ""
  };
}

function truncateText(value, maxLength) {
  const text = String(value);
  return text.length <= maxLength ? text : `${text.slice(0, maxLength)}...`;
}

function sha256Short(text) {
  return crypto.createHash("sha256").update(text).digest("hex").slice(0, 16);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function apiJSON(pathname, init = {}) {
  const response = await fetch(`${apiBase}${pathname}`, {
    ...init,
    headers: {
      authorization: authorizationHeader,
      accept: "application/json",
      ...(init.body ? { "content-type": "application/json" } : {}),
      ...(init.headers || {})
    }
  });
  const bodyText = await response.text();
  if (!response.ok) {
    throw new Error(`${pathname} returned HTTP ${response.status}: ${truncateText(bodyText, 512)}`);
  }
  return bodyText ? JSON.parse(bodyText) : null;
}

async function createRoom() {
  if (existingSessionID) {
    proof.session_id = existingSessionID;
    proof.request.mode = "existing_session";
    addStage("using_existing_session", { session_id: existingSessionID });
    return existingSessionID;
  }
  if (!evidenceSnapshotID) {
    throw new Error("OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID is required when no session is provided");
  }
  const requestBody = { evidence_snapshot_id: evidenceSnapshotID };
  if (notificationChannelProfileID > 0) {
    requestBody.close_notification_channel_profile_id = notificationChannelProfileID;
  }
  const body = await apiJSON("/api/v1/diagnosis/rooms", {
    method: "POST",
    body: JSON.stringify(requestBody)
  });
  if (!body.session_id) {
    throw new Error("create diagnosis room response missing session_id");
  }
  proof.session_id = body.session_id;
  proof.request.mode = "create_room";
  proof.created_room = {
    session_id: body.session_id,
    diagnosis_task_id: body.diagnosis_task_id,
    workflow_id: body.workflow_id,
    run_id: body.run_id
  };
  addStage("room_created", proof.created_room);
  return body.session_id;
}

async function issueTicket(sessionID) {
  const body = await apiJSON("/api/v1/diagnosis/ws-ticket", {
    method: "POST",
    body: JSON.stringify({ session_id: sessionID })
  });
  if (!body.ticket) {
    throw new Error("diagnosis WebSocket ticket response missing ticket");
  }
  return body.ticket;
}

function websocketURL(sessionID, ticket) {
  return `${apiBase.replace(/^http/, "ws")}/ws/diagnosis?session_id=${encodeURIComponent(sessionID)}&ticket=${encodeURIComponent(ticket)}`;
}

class WebSocketOperationTimeout extends Error {
  constructor(timeoutMS) {
    super(`websocket operation timed out after ${timeoutMS}ms`);
    this.code = "websocket_timeout";
    this.timeoutMS = timeoutMS;
  }
}

function isWebSocketTimeout(error) {
  return error?.code === "websocket_timeout";
}

async function withWebSocket(sessionID, handler, timeoutMS = turnTimeoutMS) {
  const ticket = await issueTicket(sessionID);
  const ws = new WebSocket(websocketURL(sessionID, ticket));
  return await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      try {
        ws.close();
      } catch {}
      reject(new WebSocketOperationTimeout(timeoutMS));
    }, timeoutMS);
    let settled = false;
    function finish(value) {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timeout);
      try {
        ws.close();
      } catch {}
      resolve(value);
    }
    function fail(error) {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timeout);
      try {
        ws.close();
      } catch {}
      reject(error);
    }
    ws.addEventListener("open", () => handler.onOpen?.(ws));
    ws.addEventListener("error", (event) => fail(new Error(`ws error ${event.message || ""}`)));
    ws.addEventListener("message", (event) => {
      let frame;
      try {
        frame = JSON.parse(String(event.data));
      } catch (error) {
        fail(error);
        return;
      }
      handler.onFrame?.(frame, ws, finish, fail);
    });
    ws.addEventListener("close", () => {
      if (settled) {
        return;
      }
      if (handler.onClose) {
        handler.onClose(finish, fail);
        return;
      }
      fail(new Error("websocket closed before operation completed"));
    });
  });
}

function diagnosisRoomSummaryPath(sessionID) {
  return `/api/v1/diagnosis/rooms/${encodeURIComponent(sessionID)}`;
}

const acceptedNotificationProviderStatuses = new Set(["accepted", "delivered", "sent", "success"]);

function acceptedAINotification(entry) {
  if (!entry || typeof entry !== "object") {
    return false;
  }
  if (![
    "diagnosis_room.assistant_turn_notification_sent",
    "diagnosis_room.final_ready_notification_sent",
    "diagnosis_room.close_notification_sent"
  ].includes(entry.event_kind)) {
    return false;
  }
  const status = String(entry.provider_status || "").trim().toLowerCase();
  if (!status || ["failed", "error"].includes(status)) {
    return false;
  }
  if (notificationChannelProfileID > 0 && entry.notification_channel_profile_id !== notificationChannelProfileID) {
    return false;
  }
  if (entry.content_kind !== notificationProofExpectedContentKind(entry.event_kind)) {
    return false;
  }
  if (!/^[a-f0-9]{64}$/.test(String(entry.content_sha256 || ""))) {
    return false;
  }
  return acceptedNotificationProviderStatuses.has(status);
}

function notificationProofExpectedContentKind(eventKind) {
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

function notificationProofEntry(entry) {
  return {
    event_kind: entry.event_kind,
    notification_channel_profile_id: entry.notification_channel_profile_id || null,
    provider_status: entry.provider_status,
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

const requiredNotificationProofEvents = [
  "diagnosis_room.assistant_turn_notification_sent",
  "diagnosis_room.final_ready_notification_sent",
  "diagnosis_room.close_notification_sent"
];

function notificationProofHasRequiredEvents(entries) {
  const seen = new Set(entries.map((entry) => entry.event_kind));
  return requiredNotificationProofEvents.every((eventKind) => seen.has(eventKind));
}

async function waitForNotificationProof(sessionID) {
  if (notificationChannelProfileID <= 0) {
    return {
      checked_at: canonicalUTCNow(),
      requested: false,
      passed: true,
      skipped_reason: "notification_channel_not_requested",
      entries: []
    };
  }
  const started = Date.now();
  let lastTimeline = [];
  while (Date.now() - started <= notificationProofTimeoutMS) {
    const summary = await apiJSON(diagnosisRoomSummaryPath(sessionID));
    const timeline = Array.isArray(summary.notification_timeline)
      ? summary.notification_timeline
      : [];
    lastTimeline = timeline;
    const entries = timeline.filter(acceptedAINotification).map(notificationProofEntry);
    if (notificationProofHasRequiredEvents(entries)) {
      return {
        checked_at: canonicalUTCNow(),
        requested: true,
        passed: true,
        entries
      };
    }
    await sleep(notificationProofPollMS);
  }
  return {
    checked_at: canonicalUTCNow(),
    requested: true,
    passed: false,
    skipped_reason: "ai_notification_content_proof_not_found",
    entries: lastTimeline.map(notificationProofEntry)
  };
}

async function queryState(sessionID) {
  return await withWebSocket(sessionID, {
    onOpen(ws) {
      ws.send(JSON.stringify({ type: "query_state" }));
    },
    onFrame(frame, _ws, finish, fail) {
      if (frame.type === "ready") {
        return;
      }
      if (frame.type === "state") {
        finish(frame);
        return;
      }
      if (frame.type === "error") {
        fail(new Error(`state query error: ${frame.code || ""} ${frame.message || ""}`));
      }
    }
  }, Math.min(turnTimeoutMS, 30000));
}

async function submitAndWait(sessionID, frame, label) {
  const initial = await queryState(sessionID);
  const initialTurn = initial.turn_count || 0;
  addStage(`${label}_before`, sanitizeFrame(initial));
  try {
    return await withWebSocket(sessionID, {
      onOpen(ws) {
        ws.send(JSON.stringify(frame));
      },
      onFrame(inbound, ws, finish, fail) {
        if (inbound.type === "ready") {
          return;
        }
        addStage(`${label}_frame`, sanitizeFrame(inbound));
        if (inbound.type === "error") {
          fail(new Error(`${label} error: ${inbound.code || ""} ${inbound.message || ""}`));
          return;
        }
        if (inbound.type === "turn_result") {
          ws.send(JSON.stringify({ type: "query_state" }));
          return;
        }
        if (inbound.type === "state" && (inbound.turn_count || 0) > initialTurn && inbound.in_flight === false) {
          finish(inbound);
        }
      }
    });
  } catch (error) {
    if (!isWebSocketTimeout(error)) {
      throw error;
    }
    return await recoverTimedOutTurn(sessionID, label, initialTurn, error.timeoutMS);
  }
}

async function recoverTimedOutTurn(sessionID, label, initialTurn, timeoutMS) {
  addStage(`${label}_timeout_recovery`, {
    timeout_ms: timeoutMS,
    recovery_timeout_ms: timeoutRecoveryMS,
    poll_ms: timeoutRecoveryPollMS
  });
  const started = Date.now();
  let lastError = null;
  let lastState = null;
  while (Date.now() - started <= timeoutRecoveryMS) {
    try {
      const state = await queryState(sessionID);
      lastState = state;
      if ((state.turn_count || 0) > initialTurn && state.in_flight === false) {
        addStage(`${label}_frame`, sanitizeFrame(state));
        return state;
      }
    } catch (error) {
      lastError = error;
    }
    await sleep(timeoutRecoveryPollMS);
  }
  if (lastState) {
    addStage(`${label}_timeout_recovery_exhausted`, sanitizeFrame(lastState));
  }
  const detail = lastError ? `; last recovery error: ${truncateText(lastError.message, 256)}` : "";
  throw new Error(`${label} did not complete after websocket timeout recovery${detail}`);
}

async function waitForIdleState(sessionID, state, label) {
  if (!state || state.status === "closed" || state.in_flight !== true) {
    return state;
  }
  addStage(`${label}_waiting_for_idle`, sanitizeFrame(state));
  const started = Date.now();
  let lastState = state;
  let lastError = null;
  while (Date.now() - started <= timeoutRecoveryMS) {
    try {
      const current = await queryState(sessionID);
      lastState = current;
      if (current.status === "closed" || current.in_flight === false) {
        addStage(`${label}_idle`, sanitizeFrame(current));
        return current;
      }
    } catch (error) {
      lastError = error;
    }
    await sleep(timeoutRecoveryPollMS);
  }
  addStage(`${label}_idle_timeout`, sanitizeFrame(lastState));
  const detail = lastError ? `; last state error: ${truncateText(lastError.message, 256)}` : "";
  throw new Error(`${label} remained in-flight after ${timeoutRecoveryMS}ms${detail}`);
}

function collectionResultRequest(result) {
  return result.request || result;
}

function evidenceRequestIdentity(request) {
  return JSON.stringify({
    tool: request.tool || "",
    template_id: request.template_id || 0,
    alert_source_profile_id: request.alert_source_profile_id || 0,
    query: request.query || "",
    window_seconds: request.window_seconds || 0,
    step_seconds: request.step_seconds || 0,
    limit: request.limit || 0
  });
}

function pendingEvidenceRequests(state) {
  const results = new Set((state.evidence_collection_results || []).map((result) =>
    evidenceRequestIdentity(collectionResultRequest(result))
  ));
  return (state.evidence_requests || []).filter((request) => !results.has(evidenceRequestIdentity(request)));
}

function firstMissingEvidenceRequest(state) {
  const missing = state.consultation_insight?.missing_evidence_requests || [];
  return missing[0] || {
    label: "Residual operator evidence boundary",
    detail: "Operator acceptance of residual uncertainty after executable evidence collection.",
    priority: "high"
  };
}

function collectionResultNeedsReview(result) {
  const status = String(result?.status || "").toLowerCase();
  return status === "failed" || status === "skipped" || status === "unsupported";
}

function firstCollectionResultNeedingReview(state) {
  return (state.evidence_collection_results || []).find(collectionResultNeedsReview) || null;
}

function collectionResultSupplementalRequest(result) {
  const request = collectionResultRequest(result);
  const tool = String(result?.tool || request.tool || "planned").trim() || "planned";
  const reason = String(result?.message || result?.reason_code || request.reason || "Collection did not produce executable evidence.").trim();
  return {
    label: `${tool} evidence collection`,
    detail: reason,
    priority: "high",
    collection_result: true
  };
}

function supplementalEvidenceText(request) {
  if (supplementalTextOverride) {
    return supplementalTextOverride;
  }
  if (supplementalTemplate) {
    return supplementalTemplate
      .replaceAll("{label}", request.label || "requested evidence")
      .replaceAll("{detail}", request.detail || "not provided")
      .replaceAll("{priority}", request.priority || "unknown");
  }
  if (request.collection_result) {
    return [
      `Operator reviewed the ${request.label} result.`,
      `Collection result detail: ${request.detail}`,
      "The skipped, failed, or unsupported executable evidence path is accepted as residual uncertainty for this live validation window.",
      "Do not fabricate unavailable metric facts. Use the collected evidence and this operator review to keep or produce a bounded ready_for_review conclusion with requires_human_review=true if no additional executable evidence is available."
    ].join("\n");
  }
  return [
    `Operator reviewed the requested follow-up for ${request.label}.`,
    "The requested operator or DBA artifact is not available in this live validation window.",
    "Operator accepts this as residual uncertainty for review purposes; do not fabricate missing facts and do not repeat this same artifact request.",
    "Use the collected executable evidence plus this boundary note to produce a bounded ready_for_review conclusion with requires_human_review=true if no additional executable evidence is needed."
  ].join("\n");
}

function readyForReview(frame) {
  const summary = sanitizeFrame(frame);
  return summary.conclusion_status === "ready_for_review" || summary.final_conclusion_available;
}

async function main() {
  validateBaseURL(apiBase);
  const sessionID = await createRoom();
  let state = await queryState(sessionID);
  addStage("initial_state", sanitizeFrame(state));

  if ((state.turn_count || 0) === 0 && state.status !== "closed") {
    state = await submitAndWait(sessionID, {
      type: "submit_turn",
      message_id: `direct-ws-initial-${Date.now()}`,
      message: initialMessage
    }, "initial_turn");
  }
  state = await waitForIdleState(sessionID, state, "initial_state");

  const pending = pendingEvidenceRequests(state);
  if (collectPlannedEvidence && pending.length > 0 && state.status !== "closed") {
    const requests = pending.slice(0, 3);
    addStage("collecting_evidence", {
      request_count: requests.length,
      tools: requests.map((request) => request.tool)
    });
    state = await submitAndWait(sessionID, {
      type: "collect_evidence",
      message_id: `direct-ws-collect-${Date.now()}`,
      message: [
        "Run planned evidence collection.",
        "After collecting this evidence, reassess confidence and state whether the conclusion is ready for operator review.",
        "If only non-executable operator or DBA evidence remains unavailable, keep requires_human_review true and use ready_for_review with explicit caveats instead of repeating the same request."
      ].join("\n"),
      evidence_requests: requests
    }, "collect_turn");
  }

  const collectionResultForReview = firstCollectionResultNeedingReview(state);
  const missingEvidenceForReview = state.consultation_insight?.missing_evidence_requests?.[0] || null;
  if (
    submitSupplementalEvidence &&
    (!readyForReview(state) || collectionResultForReview || missingEvidenceForReview) &&
    state.status !== "closed"
  ) {
    const request = collectionResultForReview
      ? collectionResultSupplementalRequest(collectionResultForReview)
      : (missingEvidenceForReview || firstMissingEvidenceRequest(state));
    const evidence = supplementalEvidenceText(request);
    addStage("submitting_supplemental_boundary", {
      label: request.label,
      priority: request.priority,
      collection_result: request.collection_result === true,
      evidence_sha256_16: sha256Short(evidence)
    });
    state = await submitAndWait(sessionID, {
      type: "submit_supplemental_evidence",
      message_id: `direct-ws-supplemental-${Date.now()}`,
      message: [
        "Supplemental evidence update",
        "",
        `Request: ${request.label}`,
        `Priority: ${request.priority}`,
        `Requested detail: ${request.detail}`,
        "",
        "Evidence provided:",
        evidence,
        "",
        "Review instruction:",
        "If this evidence directly satisfies the request, or explicitly states that the requested artifact is unavailable and the operator accepts the residual uncertainty, do not ask for the same artifact again. Produce a bounded ready_for_review conclusion with requires_human_review=true and confidence medium or high when no additional executable evidence is needed. Keep final reserved for conclusions without unresolved evidence. Low confidence or needs_evidence output must include a concrete next evidence path."
      ].join("\n"),
      supplemental_evidence: {
        label: String(request.label),
        detail: String(request.detail),
        priority: String(request.priority || "high").toLowerCase(),
        evidence
      }
    }, "supplemental_turn");
  } else if (submitSupplementalEvidence && readyForReview(state)) {
    addStage("supplemental_skipped_ready_for_review", {
      reason: "planned_evidence_already_converged",
      turn_count: sanitizeFrame(state).turn_count,
      confidence: sanitizeFrame(state).confidence,
      collection_results: sanitizeFrame(state).collection_results,
      evidence_requests: sanitizeFrame(state).evidence_requests,
      missing: sanitizeFrame(state).missing
    });
  }

  proof.final_state = sanitizeFrame(state);
  const createdRoom = proof.request.mode === "create_room";
  const shouldConfirm = confirmConclusionEnv || createdRoom;
  proof.request.confirm_conclusion_requested = shouldConfirm;
  if (readyForReview(state) && shouldConfirm && state.status !== "closed") {
    const confirmed = await submitConfirm(sessionID);
    proof.confirmation = {
      checked_at: canonicalUTCNow(),
      requested: true,
      final_state: sanitizeFrame(confirmed),
      passed: confirmed.status === "closed" &&
        confirmed.final_conclusion?.status === "available" &&
        confirmed.close_reason === "human_confirmed"
    };
  } else {
    proof.confirmation = {
      checked_at: canonicalUTCNow(),
      requested: shouldConfirm,
      skipped_reason: shouldConfirm ? "diagnosis_not_ready_for_confirmation" : "confirmation_not_requested",
      passed: !shouldConfirm
    };
  }
  proof.notification_proof = await waitForNotificationProof(sessionID);
  addStage("notification_proof", {
    requested: proof.notification_proof.requested,
    passed: proof.notification_proof.passed,
    entry_count: proof.notification_proof.entries.length
  });
  proof.passed = readyForReview(state) &&
    proof.confirmation.passed === true &&
    proof.notification_proof.passed === true;
  addStage("proof_written", {
    output_path: outputPath,
    passed: proof.passed,
    final_state: proof.final_state,
    confirmation: proof.confirmation,
    notification_proof: proof.notification_proof
  });
  writeProof();
  if (!proof.passed) {
    process.exitCode = 3;
  }
}

async function submitConfirm(sessionID) {
  return await withWebSocket(sessionID, {
    onOpen(ws) {
      ws.send(JSON.stringify({ type: "confirm_conclusion", reason: "human_confirmed" }));
    },
    onFrame(frame, _ws, finish, fail) {
      if (frame.type === "ready") {
        return;
      }
      addStage("confirm_frame", sanitizeFrame(frame));
      if (frame.type === "error") {
        fail(new Error(`confirm error: ${frame.code || ""} ${frame.message || ""}`));
        return;
      }
      if (frame.type === "state" && frame.status === "closed") {
        finish(frame);
      }
    }
  }, Math.min(turnTimeoutMS, 60000));
}

function validateBaseURL(raw) {
  let url;
  try {
    url = new URL(raw);
  } catch {
    throw new Error("OPENCLARION_LIVE_API_BASE_URL must be a valid URL");
  }
  if (!["http:", "https:"].includes(url.protocol)) {
    throw new Error("OPENCLARION_LIVE_API_BASE_URL must use http or https");
  }
  if (url.username || url.password || url.search || url.hash) {
    throw new Error("OPENCLARION_LIVE_API_BASE_URL must not include userinfo, query, or fragment");
  }
}

main().catch((error) => {
  proof.error = truncateText(error?.message || String(error), 1000);
  proof.passed = false;
  writeProof();
  console.error(`[diagnosis-live-convergence-smoke] ${proof.error}`);
  console.error(`[diagnosis-live-convergence-smoke] proof: ${outputPath}`);
  process.exit(1);
});
EOF

go run ./scripts/diagnosis_live_convergence_smoke_output "$output"
echo "[diagnosis-live-convergence-smoke] OK - live convergence output: $output" >&2
