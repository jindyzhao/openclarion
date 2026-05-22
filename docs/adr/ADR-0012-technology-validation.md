---
status: "proposed"
date: 2026-05-19
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0012: Technology Stack Validation

> **Review Period**: Until 2026-05-21 (48-hour minimum)

## Context and Problem Statement

OpenClarion has selected its technology stack across multiple ADRs (0001, 0003,
0004, 0007, 0008, 0009, 0010, 0011). Before implementation begins, the project
must validate that each choice is production-viable, identify risks, and confirm
alignment with industry best practices as of mid-2026.

## Decision Drivers

* confirm all runtime dependencies are released and actively maintained
* identify experimental or pre-v1 components and document fallback paths
* validate that no unnecessary framework dependencies exist
* ensure version selections align with LTS and support timelines
* record the validation date for future re-assessment

## Validation Results

| Component | Version | Release Date | Status | Risk |
|-----------|---------|--------------|--------|------|
| Go | 1.25+ | 2025-08-12 | maintenance (1.26 released 2026-02) | low |
| PostgreSQL | 18 | 2025-09-25 | current stable | low |
| Temporal Go SDK | >= 1.21 pinned at M0 | continuous | active | low |
| Ent ORM | TBD pinned at M0 | continuous | active (Linux Foundation, maintained by Atlas Team) | low |
| Atlas | TBD pinned at M0 | continuous | active (Ariga) | low |
| oapi-codegen-exp | V3 pre-release (pinned commit) | experimental | pre-v1, based on libopenapi | medium |
| Node.js | 24.x LTS | 2025-05 (LTS Oct 2025) | Active LTS | low |
| React | 19 | 2024-12 | current stable | low |
| Next.js | 16 | 2025 | current stable | low |
| OpenTelemetry Go | TBD pinned at M0 | continuous | CNCF graduated | low |
| pgvector | 0.7+ | 2024 | active | low (future, not MVP) |

## Decision Outcome

**Chosen option**: accept the validated stack with the following amendments.

### Amendment 1: Remove Gin Framework

**Rationale**: Go 1.22+ introduced enhanced `net/http` routing patterns that
eliminate the need for a third-party HTTP framework. Since oapi-codegen-exp
generates `ServerInterface` implementations using std `net/http`, adding Gin
would:

* introduce an unnecessary runtime dependency
* conflict with the project's minimal-dependency principle (ADR-0001)
* require adapter code between Gin and generated handlers

The std HTTP approach is explicitly recommended by the oapi-codegen maintainers
for Go 1.22+ projects.

### Amendment 2: Update Node.js to 24.x LTS

**Rationale**: Node.js 22 entered Active LTS in October 2024 and transitioned to
maintenance in October 2025. Node.js 24, released May 2025, is the current
Active LTS with support through October 2026 and maintenance through April 2028.
Starting a new project on a maintenance-phase runtime is inadvisable when the
successor LTS is stable. Pin to `24.x` to avoid accidentally running on
non-LTS odd-numbered releases.

### Amendment 3: Document oapi-codegen-exp Risk

**Rationale**: `oapi-codegen-exp` is the only Go code generator with native
OpenAPI 3.1 support. However, it is pre-v1 and explicitly experimental. The risk
is acceptable because:

* the project pins to a specific commit hash
* CI validates generated code compiles on every PR
* a documented fallback path exists (v2 + compatibility bridge)
* the experimental version is maintained by the same team as stable v2

See ADR-0007 for detailed mitigation strategy.

### Amendment 4: Replace OpenClaw Binding with Generic Agent Sandbox

**Rationale**: OpenClaw is a standalone AI agent gateway designed for interactive
use (CLI + messaging channels). Integrating it as a control-plane dependency
would require Gateway RPC integration, adding MVP complexity without
proportional value. The project's actual need is:

* M2: direct LLM API call (no agent runtime needed)
* M4: a Docker sandbox that runs any analysis script and returns structured JSON

Rather than binding to a specific agent runtime, the project defines a generic
`ContainerProvider` interface. The sandbox interior can be:

* a Python script with LLM API access and tool helpers
* a Go binary with predefined analysis steps
* any agent framework (LangChain, CrewAI, or future runtimes)
* OpenClaw itself, if later evaluation shows it fits

This preserves maximum flexibility while eliminating a pre-MVP dependency on an
emerging project with uncertain headless-integration ergonomics.

### Amendment 5: Temporal Go SDK Minimum Version

**Rationale**: M5 interactive diagnosis requires Workflow Update for synchronous
request-response semantics between the WebSocket handler and running workflows
(see ADR-0004). Workflow Update is available in Temporal Go SDK >= 1.21 and
requires a compatible Temporal Server version.

* Pin SDK to >= 1.21 in `go.mod`
* Validate Update round-trip in M0 integration test
* Temporal Server (via temporalite in dev, self-hosted in prod) must also
  support Update; version verified during M0 bootstrap

### Consequences

* Good, because all dependencies are confirmed released and maintained.
* Good, because removing Gin reduces runtime dependency surface.
* Good, because Node.js 24.x LTS aligns with Active LTS timelines.
* Good, because generic sandbox interface preserves runtime flexibility.
* Good, because Temporal selection is now explicitly tied to M5 short-
  conversation diagnosis (see ADR-0004 re-evaluation trigger).
* Good, because Temporal Update requirement is explicitly version-constrained.
* Neutral, because oapi-codegen-exp requires version pinning discipline.
* Neutral, because M5 is V1 required at minimum-viable scope; long-session
  features are deferred.

### Confirmation

* Temporal Go SDK is pinned to >= 1.21 in `go.mod`
* M0 integration test validates Temporal Update round-trip
* `go.mod` does not import Gin, Echo, or Fiber
* `package.json` specifies Node.js 24.x LTS engine requirement
* `oapi-codegen-exp` is pinned to a commit hash, not `@latest`
* generated server code uses std `net/http`
* no specific agent runtime is imported for M0-M2 code paths
* `ContainerProvider` interface accepts any OCI-compliant runtime

## Best Practices Alignment

| Practice | Source | Alignment |
|----------|--------|-----------|
| single database for MVP | PostgreSQL community, Temporal docs | aligned (ADR-0001) |
| contract-first API | OpenAPI Initiative, oapi-codegen docs | aligned (ADR-0007) |
| durable workflows over state machines | Temporal best practices | aligned (ADR-0004) |
| AI as black-box runtime | emerging MLOps patterns | aligned (ADR-0002) |
| generic sandbox over specific runtime | flexibility principle | aligned (this ADR) |
| ephemeral non-root containers | CIS benchmarks, Docker security | aligned (ADR-0005) |
| monorepo for atomic changes | Google/Meta engineering practices | aligned (ADR-0008) |
| std HTTP over framework | Go community consensus post-1.22 | aligned (this ADR) |

## Re-assessment Schedule

This validation should be re-assessed:

* before M1 milestone begins (control plane implementation)
* if any pinned dependency releases a breaking change
* every 6 months from the validation date

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-19 | jindyzhao | Initial validation |
| 2026-05-19 | jindyzhao | Replace OpenClaw binding with generic agent sandbox |
| 2026-05-19 | jindyzhao | Tie Temporal selection to M5 V1 commitment; soften Structured Outputs language; reword OpenClaw integration cost |
| 2026-05-19 | jindyzhao | Add Temporal SDK >= 1.21 constraint for Workflow Update support |
