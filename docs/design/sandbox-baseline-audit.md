# Sandbox Baseline Audit

OpenClarion's current product path remains intelligent alert analysis. The M4
sandbox baseline exists to make alert report enhancement and the later M5
short-conversation diagnosis safer to evaluate without accepting a specific
agent framework in the Go control plane.

`scripts/sandbox_baseline_audit` is a code-level audit helper. It does not
start Docker and does not prove live daemon cleanup by itself. Instead, it
calls the same provider-neutral and Docker-spec code used by the sandbox path
and emits one JSON object proving the current baseline invariants:

- fixed ADR-0013 file paths for evidence, optional M5 conversation/message
  inputs, agent config, and `/workspace/out/output.json`
- batch sandbox runs default to network-none
- M5 turn inputs mount read-only when present
- Docker runtime spec remains non-root, readonly-rootfs, no-new-privileges,
  unprivileged, capability-dropped, and resource-limited
- allowlist-mode requests use the dedicated allowlist network and a static
  subset enforcer rejects drift before Docker create
- raw container output is validated as bounded JSON before report-specific
  schema validation

The CI target is:

```bash
make sandbox-baseline-audit
```

For retained M4 decision evidence, operators can write the same audit output
to a new file:

```bash
make sandbox-m4-baseline-audit \
  OUT=artifacts/m4/baseline-audit.json
```

The retained output path must not already exist. The manual target preserves
the CI target's checks; it only changes where the JSON evidence is written.

This audit complements, but does not replace, the manual Docker smoke targets:

- `make container-provider-smoke`
- `make container-provider-timeout-smoke`
- `make container-provider-output-cap-smoke`
- `make egress-allowdeny-smoke`
- `make custom-thin-runner-smoke`

Together, the code-level audit plus manual smokes are the evidence path for the
minimum M4/M5 sandbox baseline. Real report-quality acceptance remains separate
and depends on representative alert evidence and direct-vs-sandbox report
outputs.

## Current Evidence

As of 2026-05-29, the M5 minimum sandbox baseline has local evidence from:

- `make sandbox-baseline-audit`
- `make custom-thin-runner-smoke`
- `make container-provider-smoke`
- `make container-provider-timeout-smoke`
- `make container-provider-output-cap-smoke`
- `make egress-allowdeny-smoke`

This proves the minimum file-based invocation contract, packaged helper
execution, timeout cleanup, output-size enforcement, and proxy allow/deny
behavior needed before M5 implementation work starts. The runtime-agnostic
Docker sandbox baseline is closed; the M4 report-enhancement track and any
specific OpenClaw/Hermes framework baseline still require representative
quality evidence and a recorded decision.
