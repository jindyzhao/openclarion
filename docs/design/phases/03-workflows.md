# Phase 03: Workflows

## Goal

Implement durable orchestration for evidence snapshots, report generation,
notifications, retries, and failure marking.

## Workflows

| Workflow | Purpose |
|----------|---------|
| DiagnosisWorkflow | process one evidence snapshot |
| ReportFanOutWorkflow | generate per-group subreports |
| ReportBatchWorkflow | fan out subreports and fan in to a final report |
| FinalReportWorkflow | reduce subreports and persist final report |
| WeeklyReportWorkflow | future scheduled summary |

## Workflow Policies

Runtime workflow selection is governed by backend-owned policy profiles, not by
frontend-local state. See
[ADR-0014](../../adr/ADR-0014-alert-operations-configuration.md).

Policy-driven trigger inputs must bind to immutable identifiers:

- alert source profile ID
- grouping policy ID
- report workflow policy ID
- notification channel profile ID when notification delivery is enabled

Temporal workflow code must not read mutable configuration directly during
deterministic replay. Workflow starts should receive resolved identifiers and
request metadata, while Activities load provider details when performing
external I/O.

## Activity Rules

- external I/O belongs in activities
- workflows contain deterministic orchestration only
- retries and timeouts are explicit
- workflow IDs include stable business identifiers
- usecases start workflows through ports; Temporal client types stay outside
  `internal/usecases`
- workflow policy previews and dry-runs must be explicit backend operations
  before enablement

## Acceptance

- workflow tests cover success, timeout, provider failure, and retry
- failure states persist to `DiagnosisTask`
- reports link back to `EvidenceSnapshot`
- report triggers can bind to configured policy identifiers without hard-coded
  customer endpoints
