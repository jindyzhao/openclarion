# OpenClarion

> Natural-language alert governance for evidence capture, accountable response,
> AI-assisted analysis, and post-incident reporting.
>
> Author: jindyzhao

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Project Status](https://img.shields.io/badge/status-private%20incubation-orange.svg)](#project-status)

OpenClarion is a private-incubation open source project. The repository is kept
private while the product position, minimum viable scope, and early integration
contracts are still being validated. The project is prepared with open source
license, governance, security, contribution, ADR, and documentation standards so
it can move to a public repository when the position is mature.

## What OpenClarion Does

OpenClarion turns alert response into a governed, auditable workflow:

1. Read active alerts from Prometheus-compatible systems or Alertmanager.
2. Deduplicate and group alerts by deterministic dimensions.
3. Build an `EvidenceSnapshot` with alert labels, metrics, topology, ownership,
   and recent context.
4. Generate structured `SubReport` and `FinalReport` output through a headless
   LLM report loop.
5. Distribute concise reports through IM providers.
6. Persist evidence, tasks, chat turns, and reports in PostgreSQL for review,
   weekly/monthly summaries, and future retrieval.

OpenClarion does **not** replace realtime paging, silencing, inhibition, or on-call
routing. Existing realtime alert channels remain the source of immediate human
notification. OpenClarion focuses on governance: response, escalation, evidence,
review, and accountability.

## Early Delivery Cutline

The project is feasible, but the work must be sequenced carefully. The Go
control plane and headless LLM report loop are deterministic enough to implement
first. Interactive OpenClaw sessions are valuable but substantially harder.

| Priority | Scope | Risk | Delivery intent |
|----------|-------|------|-----------------|
| P0 | Go control plane: alert reading, sharding, grouping, evidence snapshots, Temporal workflows | Medium | First implementation target |
| P1 | Headless LLM reports: structured `SubReport` and `FinalReport`, persistence, notification | Medium | First value proof |
| P2 | OpenClaw headless sandbox: readonly skills, fixed timeout, stdout JSON, cleanup | Medium-high | Parallel proof-of-concept |
| P3 | OpenClaw interactive diagnosis room: WebSocket/PTY, identity, RBAC, audit, lifecycle compression | High | Later track, after P0/P1 are stable |

## Architecture

```text
Prometheus / Alertmanager
        |
        v
Go Control Plane  ----> PostgreSQL
        |                  ^
        |                  |
        v                  |
Temporal Workflows --------+
        |
        +--> Headless LLMProvider reports
        |
        +--> OpenClaw sandbox proof-of-concept
        |
        +--> IM Provider distribution
```

### Core Principles

- **Deterministic control flow**: Go and Temporal own routing, sharding,
  retries, escalation, and lifecycle decisions.
- **PostgreSQL as the source of truth**: business state, evidence, reports, and
  audit records live in PostgreSQL.
- **Provider extension seams**: Metrics, CMDB, IM, Auth, Approval, Container,
  and LLM integrations are interfaces, not hard-coded systems.
- **Agent as a sandboxed worker**: OpenClaw is used as a black-box runtime behind
  strict sandbox, tool, network, timeout, and output boundaries.
- **Contract-first API**: OpenAPI 3.1 is the canonical API contract.
- **Frontend in the monorepo**: the future web console lives under `web/` so API,
  generated types, backend, and UI changes stay atomic.

## Technology Baseline

| Area | Baseline |
|------|----------|
| Backend | Go 1.25+, Gin |
| Database | PostgreSQL 18, Ent, Atlas migrations |
| Workflow | Temporal Go SDK |
| API | OpenAPI 3.1, `oapi-codegen-exp` |
| Frontend | React, Next.js, generated API types |
| AI runtime | Headless LLMProvider first; OpenClaw sandbox later |
| Observability | OpenTelemetry, Prometheus metrics |
| CI | license, DCO, lint, tests, API generation, docs checks, no non-English literals outside approved paths |

## Repository Layout

```text
api/                 OpenAPI contracts
cmd/                 service entrypoints
internal/            private application code
pkg/                 public extension interfaces and SDK surfaces
web/                 frontend application
ent/                 Ent schemas and generated code
migrations/          Atlas migrations
docs/                project documentation, ADRs, design specs
.github/             issue templates, pull request template, workflows
scripts/             repository maintenance checks
```

## Documentation

- [Documentation index](docs/README.md)
- [Architecture decisions](docs/adr/README.md)
- [Design overview](docs/design/README.md)
- [Roadmap](docs/roadmap/tasks.md)
- [Development workflow](DEVELOPMENT_WORKFLOW.md)
- [Contributing](CONTRIBUTING.md)
- [Governance](GOVERNANCE.md)
- [Security policy](SECURITY.md)

## Project Status

OpenClarion is in private incubation. The current goal is to validate the P0/P1
cutline with replayed alert windows before moving interactive OpenClaw sessions
onto the critical path.

## Community and Governance

The repository already carries the governance documents required for public
operation:

- Apache-2.0 license
- Contributor Covenant-based code of conduct
- Developer Certificate of Origin
- maintainer and contributor roles
- security vulnerability reporting process
- ADR lifecycle and documentation governance

## License

Apache License 2.0. See [LICENSE](LICENSE).

Copyright The OpenClarion Authors.
