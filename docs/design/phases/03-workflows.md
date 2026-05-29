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

## Activity Rules

- external I/O belongs in activities
- workflows contain deterministic orchestration only
- retries and timeouts are explicit
- workflow IDs include stable business identifiers
- usecases start workflows through ports; Temporal client types stay outside
  `internal/usecases`

## Acceptance

- workflow tests cover success, timeout, provider failure, and retry
- failure states persist to `DiagnosisTask`
- reports link back to `EvidenceSnapshot`
