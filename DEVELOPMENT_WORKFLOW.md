# Development Workflow

This document is the working guide for local development and review hygiene.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.25+ | backend development |
| Docker | 24+ | local services and sandbox tests |
| Docker Compose | v2+ | PostgreSQL and Temporal stack |
| Node.js | 22+ | frontend development |
| PostgreSQL client | 18+ | optional local database inspection |

## Local Setup

```bash
git clone git@github.com:jindyzhao/openclarion.git
cd openclarion

make dev-up
make generate
make migrate
make test
make run
```

Frontend work will live under `web/` once the frontend skeleton is created:

```bash
cd web
npm ci
npm run dev
```

## Coding Rules

| Rule | Requirement |
|------|-------------|
| Formatting | `gofmt`, `goimports`, and project lint must pass |
| Errors | every error must be handled or intentionally wrapped |
| Logging | use structured `slog`; never log secrets or raw tokens |
| Concurrency | use worker pools or Temporal activities; do not launch unmanaged goroutines |
| Data access | PostgreSQL through Ent or reviewed parameterized SQL |
| API | OpenAPI 3.1 is the canonical contract |
| Frontend | generated API types; no hand-written duplicate DTOs |

## Prohibited Patterns

| Pattern | Replacement |
|---------|-------------|
| unmanaged `go func() {}` | worker pool, errgroup with ownership, or Temporal activity |
| hand-written distributed state machine | Temporal workflow |
| direct modification of AI agent internals | prompts, skills, sandbox configuration |
| polyglot persistence for core state | PostgreSQL relational schema / JSONB / pgvector |
| privileged AI container | non-root, readonly rootfs, network allowlist, fixed timeout |
| generated-code drift | `make generate` and CI freshness checks |

## ADR Workflow

1. Copy `docs/adr/TEMPLATE.md` to `docs/adr/ADR-XXXX-short-title.md`.
2. Keep one ADR to one architectural decision.
3. Use `Proposed` status for review.
4. Do not land normative implementation based on a proposed ADR unless the PR is
   explicitly limited to review material.
5. After acceptance, implementation PRs may update design specs and code.
6. Accepted ADRs are immutable. Use a new ADR to amend or supersede them.

## Validation Levels

| Level | Command |
|-------|---------|
| Fast backend | `go test ./...` |
| Generated artifacts | `make generate && git diff --exit-code` |
| Lint | `make lint` |
| Full PR mirror | `make pr` |
| Docs hygiene | `bash scripts/check_no_non_english_chars.sh` |

## Commit Standard

Use Conventional Commits and DCO sign-off:

```text
feat(control-plane): add alert replay harness

Explain what changed and why. Wrap body text at 72 characters when practical.

Refs #123
Signed-off-by: Your Name <you@example.com>
```
