#!/usr/bin/env bash
# Reject agent-framework dependencies and hard-coded runtime-family names in
# first-party control-plane source until docs/design/agent-runtime-selection.md
# records an accepted sandbox baseline and the policy is updated intentionally.

set -euo pipefail

cd "$(dirname "$0")/.."

go run ./scripts/agent_runtime_policy_check "$@"
