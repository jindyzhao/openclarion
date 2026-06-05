---
id: ADR-0014
title: "Alert Operations Configuration"
status: "proposed"
date: 2026-06-05
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0014: Alert Operations Configuration

> **Review Period**: Until 2026-06-07 (48-hour minimum)<br>
> **Extends**: ADR-0001, ADR-0003, ADR-0007, ADR-0010

## Context and Problem Statement

OpenClarion currently supports a Prometheus-backed report trigger through
runtime environment variables. That is sufficient for local smoke evidence, but
it is not sufficient for an operations product where users must add alert
sources, test connectivity, configure grouping rules, select report workflow
behavior, and enable or disable integrations without changing code.

The product must stay alert-first. Prometheus and Alertmanager are alert source
adapters; they should feed the same `AlertEvent`, `AlertGroup`,
`EvidenceSnapshot`, report, and diagnosis-room pipeline. The frontend must not
hard-code external addresses, persist real credentials in browser storage, or
own workflow routing decisions.

## Decision Drivers

* keep alert ingestion, grouping, reporting, and diagnosis configuration
  auditable
* avoid hard-coded customer Prometheus or Alertmanager addresses in source,
  tests, fixtures, or frontend defaults
* keep secrets out of OpenAPI responses, browser state, Git history, and
  retained smoke artifacts
* preserve generated OpenAPI contracts for frontend configuration screens
* keep Temporal workflows deterministic by loading runtime decisions before
  workflow start or inside Activities
* allow provider tests to stay independent from external Prometheus or
  Alertmanager systems

## Considered Options

* **Option 1**: Keep environment-variable-only integration configuration.
* **Option 2**: Store integration configuration only in frontend local state.
* **Option 3**: Store operator-managed alert operations profiles in PostgreSQL
  and expose them through generated OpenAPI contracts.

## Decision Outcome

**Chosen option**: Option 3, because OpenClarion needs auditable, shared,
server-owned configuration while keeping provider interfaces compile-time and
testable.

### Configuration Profiles

The configuration model is split by responsibility:

| Profile | Purpose |
|---------|---------|
| `AlertSourceProfile` | Prometheus or Alertmanager connection metadata, source kind, display name, base URL, auth mode, secret reference, enabled state, and operator labels |
| `GroupingPolicy` | deterministic grouping keys, severity key, source scope, and preview metadata |
| `ReportWorkflowPolicy` | trigger mode, report scenario, grouping policy binding, notification channel binding, and diagnosis-room follow-up behavior |
| `NotificationChannelProfile` | report and close-notification delivery target metadata and secret references |

Profiles are business configuration and therefore belong in PostgreSQL per
ADR-0001. Secrets are not stored directly in these rows. Rows carry only
`secret_ref` values that point at a deployment-managed secret boundary. The API
must never return secret material.

### API and Frontend Boundary

OpenAPI remains the contract source. Configuration screens consume generated
TypeScript types and call backend APIs for list, create, update, test, preview,
and enable actions. The frontend may hold form draft state, but it must not own
the durable configuration or write workflow decisions directly.

Connectivity tests and grouping previews are explicit backend operations. They
return sanitized results and bounded samples. A successful test or preview does
not automatically enable a profile; enablement is a separate audited action.

Alert source connection tests use a dedicated action endpoint. The action reads
the persisted profile by ID, performs bounded provider I/O in backend code, and
returns only status, reason, checked time, kind, auth mode, and small counters.
It must not echo the profile base URL, raw upstream errors, bearer tokens,
secret references beyond the already-persisted profile contract, or sampled
alert payloads. Prometheus profiles are tested through the Prometheus
`/api/v1/alerts` API. Alertmanager profiles are tested through the
Alertmanager `/api/v2/alerts` API with query parameters that include active
alerts and exclude silenced, inhibited, and unprocessed alerts. Profiles with
`auth_mode=bearer` require a server-side secret resolver to exchange
`secret_ref` for a bearer token. If the resolver is not configured, or the
reference is unavailable, the action returns a blocked sanitized result rather
than constructing a provider or exposing resolver details.

Grouping policies use a dedicated profile and preview endpoint. A policy stores
the operator-facing name, deterministic alert label keys used as grouping
dimensions, the severity label key, an optional source filter, and an enabled
flag. The preview action loads the persisted policy by ID, reads only a bounded
recent `AlertEvent` sample from PostgreSQL, applies the same pure grouping
algorithm used by replay paths, and returns grouped counts, dimensions,
severity, first/last observed timestamps, and bounded event identifiers. It
must not call Prometheus or Alertmanager, persist `AlertGroup` rows, start
workflows, or treat a successful preview as enablement.

### Workflow Boundary

Workflow starts receive resolved policy identifiers and immutable request
metadata. Activities may load provider profiles by ID when performing external
I/O. Temporal workflow code must not call providers directly and must not depend
on live configuration mutation for deterministic replay.

### Consequences

* Good, because operators can manage alert sources and policies without
  rebuilding or redeploying the control plane.
* Good, because OpenAPI-generated frontend contracts keep configuration UI and
  backend validation aligned.
* Good, because tests can cover profiles with fake providers and sanitized
  example hosts instead of customer systems.
* Neutral, because the first implementation must add persistence, API, and UI
  scaffolding before the current environment-variable live-smoke path can be
  fully policy-driven.
* Bad, because profile versioning and secret reference validation add
  operational complexity that environment variables avoided.

### Confirmation

This decision is confirmed when:

* alert source profiles can be stored, listed, updated, and disabled through
  backend APIs without returning credential values
* provider connectivity tests use configured profiles without hard-coded
  customer endpoints
* grouping policies can be previewed against bounded alert samples before
  enablement
* report workflow triggers can bind to profile and policy identifiers while
  preserving the existing Prometheus live-smoke path until the policy-driven
  path has equivalent retained evidence
* frontend settings pages use generated OpenAPI types and do not persist real
  bearer tokens or URLs outside user-submitted API calls

## Pros and Cons of the Options

### Option 1: Environment-Variable-Only Configuration

* Good, because it is simple and fits manual live-smoke setup.
* Bad, because changing sources or policies requires process-level
  reconfiguration and cannot provide an audited operator UX.

### Option 2: Frontend Local Configuration

* Good, because the UI could iterate quickly.
* Bad, because browser-owned configuration cannot safely drive durable
  workflows, shared operators, secret handling, or audit trails.

### Option 3: PostgreSQL-Backed Operations Profiles

* Good, because configuration becomes auditable product state with server-side
  validation and generated API contracts.
* Good, because the same profile can drive API, CLI, scheduler, and diagnosis
  paths.
* Bad, because profile migrations and compatibility rules must be maintained.

## More Information

### Related Decisions

* ADR-0001 keeps business state in PostgreSQL.
* ADR-0003 keeps concrete provider implementations behind compile-time
  provider interfaces.
* ADR-0007 makes OpenAPI the API contract source.
* ADR-0010 requires frontend feature modules to consume generated API types.

### Implementation Notes

The first implementation slice should start with alert source profile
persistence and generated API contracts, then add an explicit connection-test
action before grouping and workflow policy screens. It should keep the existing
environment-variable Prometheus live-smoke path until the profile-driven path
has equivalent retained evidence. Alertmanager support should land as a
separate provider adapter with fake or `httptest` coverage and contract tests,
not as Prometheus-specific branching in frontend code. Secret-backed
connectivity tests require an explicit backend resolver map; they must not let
operator-submitted `secret_ref` values read arbitrary process environment
variables, and they must not expose raw secret values to OpenAPI responses,
logs, or the browser.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-06-05 | jindyzhao | Initial proposal |
| 2026-06-05 | jindyzhao | Added sanitized alert-source connection-test boundary |
| 2026-06-05 | jindyzhao | Added grouping policy persistence and dry-run preview boundary |
| 2026-06-05 | jindyzhao | Added server-side secret resolver and Alertmanager connection-test adapter boundary |
