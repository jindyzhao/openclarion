---
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0005: Ephemeral AI Container Security Model

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

AI agent runtimes can use tools and generate commands. Running
an agent directly on the host is not acceptable for production alert governance.
The project must define concrete security constraints for sandboxed containers,
including image integrity, credential management, network egress, and mount
scoping.

## Decision Drivers

* restrict filesystem and network access
* avoid host secret exposure
* guarantee cleanup after timeout or failure
* make AI tools readonly by default
* keep production-impacting actions behind human approval
* enforce supply-chain integrity (image provenance)
* minimize credential blast radius

## Decision Outcome

**Chosen option**: run AI agents only in short-lived, non-root, restricted
containers. The Go control plane owns lifecycle, input injection, output capture
(file-based), timeout, cleanup, and audit.

### Sandbox Requirements

* non-root user (`User: "nonroot"`)
* readonly root filesystem where possible
* no privileged mode
* no Docker socket mount
* no host secret mounts
* CPU and memory limits (`Resources.Memory`, `NanoCPUs`)
* `--security-opt=no-new-privileges`
* fixed lifetime and idle timeout
* structured output contract (file-based, not stdout)

### Image Integrity

* Production sandbox images must be referenced by **digest**
  (`openclarion-agent@sha256:<pinned-digest>`), never by mutable tag (`:latest`).
* Digest is pinned in deployment configuration and updated only through CI.
* CI verifies image build provenance before publishing digest.

### Credential Management

* If the agent needs API tokens (LLM API, Prometheus, K8s), Go issues
  **short-lived credentials** (TTL ≤ container timeout) injected via
  environment variables at container creation time.
* No long-lived secrets inside the container image or mounted volumes.
* Credentials expire automatically even if container cleanup fails.

### Docker Daemon Privilege Boundary

* `ContainerProvider` requires access to the Docker daemon API.
* V1: Go accesses Docker socket on the same host. The Docker socket is NOT
  mounted into sandbox containers — only the Go control plane process uses it.
* Post-V1: evaluate rootless Docker or a dedicated sandbox host with
  mTLS-protected remote API to reduce privilege surface.

### Mount Scoping

| Path | Type | Permission |
|------|------|------------|
| `/workspace/evidence.json` | bind mount | readonly |
| `/workspace/conversation.json` | bind mount | readonly (M5 only) |
| `/workspace/message.json` | bind mount | readonly (M5 only) |
| `/workspace/agent_config/` | bind mount | readonly |
| `/workspace/out/` | tmpfs | writable, size-capped (`--tmpfs /workspace/out:size=10m`) |

Agent writes ONLY to `/workspace/out/output.json`. All other filesystem paths
are read-only or inaccessible. Agent cannot write outside `/workspace/out/`.

### Network Egress Control

Docker network isolation (`--network=none`, internal networks) provides basic
containment, but **precise per-endpoint allowlist** requires additional design:

* V1: Docker internal network + host iptables for known static IPs (Prometheus).
* For SaaS LLM targets (e.g. `api.openai.com`): IP-based allowlist is fragile
  because SaaS IPs rotate. Egress proxy (Envoy/Squid with domain allowlist) is
  recommended.
* Full egress proxy required before M4 acceptance (see END_TO_END_VERIFICATION.md).

### Consequences

* Good, because agent blast radius is limited.
* Good, because cleanup is deterministic.
* Good, because image supply-chain integrity is enforced.
* Good, because credential lifetime is bounded.
* Neutral, because some skills need explicit network allowlist entries.
* Neutral, because egress proxy adds operational complexity.
* Bad, because debugging sandboxes is harder than local process execution.

### Confirmation

* container provider tests verify timeout cleanup
* sandbox configuration is visible in logs without leaking secrets
* default skills are readonly
* CI validates that production config references digest, not tag
* short-lived token TTL verified to be ≤ container timeout in tests
* integration test confirms agent cannot write outside `/workspace/out/`

## More Information

### Related Decisions

* ADR-0002 — agent black-box boundary (security is one enforcement mechanism)
* ADR-0013 — per-turn container invocation model and file-based data contract

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-05-19 | jindyzhao | Add image digest pinning, short-lived credentials, Docker daemon boundary, mount scoping table, egress proxy guidance |
