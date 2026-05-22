#!/usr/bin/env bash
set -euo pipefail

if rg --pcre2 "\p{Han}" README.md docs DEVELOPMENT_WORKFLOW.md QA.md CONTRIBUTING.md GOVERNANCE.md SECURITY.md CODE_OF_CONDUCT.md DCO.md MAINTAINERS.md ADOPTERS.md 2>/tmp/openclarion-han.txt; then
  cat /tmp/openclarion-han.txt
  echo "Non-English CJK characters found in governed documentation." >&2
  exit 1
fi
