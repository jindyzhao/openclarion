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

OpenClarion turns alert response into a governed, auditable workflow. The
current product direction is intelligent alert analysis; the architecture keeps
the alert pipeline extensible enough to accept future signal sources through
provider interfaces without changing the control plane.

1. Read active alerts from Prometheus-compatible systems, Alertmanager, or
   compatible alert-source providers.
2. Deduplicate and group alerts by deterministic dimensions.
3. Build an `EvidenceSnapshot` with alert labels, metrics, topology, ownership,
   and recent context.
4. Generate structured `SubReport` and `FinalReport` output through a headless
   LLM report loop.
5. Distribute concise reports through IM providers.
6. Persist evidence, tasks, chat turns, reports, and optional bounded historical
   report retrieval data in PostgreSQL for review and weekly/monthly summaries.

The product has two logical subsystems inside the same repository and product
surface:

- **Insight Pipeline**: the automatic alert-to-evidence-to-report path.
- **Agent Workspace**: the human-initiated diagnosis room for report follow-up,
  explanation, action drafting, and audit handoff.

They intentionally share evidence, reports, providers, auth/audit primitives,
OpenAPI, PostgreSQL, Temporal infrastructure, and observability. They do not
share workflow lifecycle, output schemas, conversation state, or
production-changing operation authority.

OpenClarion does **not** replace realtime paging, silencing, inhibition, or on-call
routing. Existing realtime alert channels remain the source of immediate human
notification. OpenClarion focuses on governance: response, escalation, evidence,
review, and accountability.

## Early Delivery Cutline

The project is feasible, but the work must be sequenced carefully. The Go
control plane and headless LLM report loop are deterministic enough to implement
first. The sandboxed Agent Workspace is valuable, but it must stay logically
separate from the automated report pipeline and behind explicit identity,
runtime, audit, and approval boundaries.

| Priority | Scope | Risk | Delivery intent |
|----------|-------|------|-----------------|
| P0 | Go control plane: alert reading, sharding, grouping, evidence snapshots, Temporal workflows | Medium | First implementation target |
| P1 | Headless LLM reports: structured `SubReport` and `FinalReport`, persistence, notification | Medium | First value proof |
| P2 | Sandboxed report enhancement: readonly tools, fixed timeout, file-contract JSON, cleanup | Medium-high | Parallel proof-of-concept |
| P3 | Agent Workspace diagnosis room: WebSocket, identity, RBAC, audit, bounded turns | High | Later track, after P0/P1 are stable |

## Architecture

```text
OpenClarion product
|-- Insight Pipeline
|   Prometheus / Alertmanager / alert-source providers
|           |
|           v
|   Go Control Plane -> Temporal report workflows -> PostgreSQL
|           |                         |
|           |                         +--> IM Provider distribution
|           +--> Headless LLMProvider reports
|           +--> Sandboxed report-enhancement runtime candidate
`-- Agent Workspace
    Browser / WebSocket -> DiagnosisRoomWorkflow -> per-turn sandbox
            |                    |
            +--------------------+--> PostgreSQL audit and chat history
```

### Core Principles

- **Deterministic control flow**: Go and Temporal own routing, sharding,
  retries, escalation, and lifecycle decisions.
- **PostgreSQL as the source of truth**: business state, evidence, reports, and
  audit records live in PostgreSQL.
- **Provider extension seams**: Metrics, alert-source, CMDB, IM, Auth,
  Approval, Container, and LLM integrations are interfaces, not hard-coded
  systems.
- **Agent as a sandboxed worker**: The M5 diagnosis runner embeds pinned Eino
  in an isolated module behind strict file, tool, network, timeout, and output
  boundaries; the Go control plane retains durable state and policy ownership.
- **Contract-first API**: OpenAPI 3.1 is the canonical API contract.
- **Frontend in the monorepo**: the web console lives under `web/` so API,
  generated types, backend, and UI changes stay atomic.

## Technology Baseline

| Area | Baseline |
|------|----------|
| Backend | Go 1.25+, std `net/http` |
| Database | PostgreSQL 18, Ent, Atlas migrations |
| Workflow | Temporal Go SDK |
| API | OpenAPI 3.1, `oapi-codegen-exp` |
| Frontend | React 19, Next.js 16, generated API types |
| AI runtime | Headless LLMProvider plus an isolated Eino `v0.9.12` diagnosis runner; production sandbox wiring remains explicit |
| Observability | OpenTelemetry, Prometheus metrics |
| CI | license, DCO, lint, tests, API generation, docs checks, no non-English literals outside approved paths |

## Repository Layout

```text
api/                 OpenAPI contracts
cmd/                 service entrypoints
internal/            private application code
web/                 frontend application
internal/persistence/ent/         Ent schemas and generated code
internal/persistence/migrations/  Atlas migrations
docs/                project documentation, ADRs, design specs
.github/             issue templates, pull request template, workflows
scripts/             repository maintenance checks
```

## Run Locally

The complete product can run without Kubernetes. Prepare a private environment
file from `.env.example`, replace its placeholders, install the locked frontend
dependencies, and run the preflight:

```bash
mkdir -p .openclarion-private
cp .env.example .openclarion-private/openclarion-stage5.env
chmod 0600 .openclarion-private/openclarion-stage5.env
npm --prefix web ci
OPENCLARION_LOCAL_PRODUCT_ENV_FILE=.openclarion-private/openclarion-stage5.env make local-product-check
```

The preflight builds the bundled diagnosis runner and egress proxy, starts the
pinned PostgreSQL/pgvector and Temporal services, applies committed Atlas
migrations, and validates runtime prerequisites. Replace `local-product-check`
with `local-product` to start the API/worker and Next.js console.

The launcher uses a dedicated Compose database by default. When
`OPENCLARION_LOCAL_USE_CONFIGURED_DATABASE=1`, Atlas still runs in a container;
a loopback database URL therefore requires either supported host networking or
a separate container-reachable `OPENCLARION_LOCAL_ATLAS_DATABASE_URL`. See
`.env.example` for both overrides.

Compose dependencies remain available after the launcher exits; stop them with:

```bash
docker compose --project-name openclarion-local-product --profile sandbox-egress down
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
intelligent alert analysis cutline with replayed alert windows while keeping
sandboxed report enhancement and Agent Workspace diagnosis behind explicit M4/M5
quality, runtime, identity, and audit gates.

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
