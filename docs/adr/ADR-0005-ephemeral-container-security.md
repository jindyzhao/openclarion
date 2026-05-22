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

## Decision Drivers

* restrict filesystem and network access
* avoid host secret exposure
* guarantee cleanup after timeout or failure
* make AI tools readonly by default
* keep production-impacting actions behind human approval

## Decision Outcome

**Chosen option**: run AI agents only in short-lived, non-root, restricted
containers. The Go control plane owns lifecycle, input injection, output capture,
timeout, cleanup, and audit.

### Sandbox Requirements

* non-root user
* readonly root filesystem where possible
* no privileged mode
* no Docker socket mount
* no host secret mounts
* network allowlist
* CPU and memory limits
* fixed lifetime and idle timeout
* structured output contract

### Consequences

* Good, because agent blast radius is limited.
* Good, because cleanup is deterministic.
* Neutral, because some skills need explicit network allowlist entries.
* Bad, because debugging sandboxes is harder than local process execution.

### Confirmation

* container provider tests verify timeout cleanup
* sandbox configuration is visible in logs without leaking secrets
* default skills are readonly

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
