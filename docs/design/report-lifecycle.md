# Alert Report Lifecycle

> Last updated: 2026-06-06
> Status: design boundary

This document defines the product and engineering boundary between automated
AI report artifacts and accountable final conclusions. OpenClarion remains an
alert-first system: alert sources feed grouped evidence, AI generates
structured reports from frozen evidence, and humans remain accountable for
business-impacting conclusions and actions.

## Lifecycle Stages

| Stage | Owner | Durable artifact | Boundary |
|-------|-------|------------------|----------|
| Alert intake | Backend adapters | `AlertEvent` | Raw alert signal ingestion. This does not create a report or a conclusion. |
| Grouping | Backend use cases | `AlertGroup` | Deterministic grouping and severity derivation. This does not call LLMs. |
| Evidence freeze | Backend use cases | `EvidenceSnapshot` | Immutable evidence package. AI output must cite or derive from this boundary. |
| Automated report | Temporal Activities + LLM provider | `SubReport` and `FinalReport` | Schema-validated AI report artifact with confidence, missing evidence, and recommended next steps. Persistence means the report is ready for review, not that the conclusion is final. |
| Human diagnosis | Operator + diagnosis room | `ChatSession`, task events, and supplemental context | Humans may ask why confidence is low, add evidence, or request more checks. The original `EvidenceSnapshot` stays immutable; supplemental context must be retained separately when it affects the final outcome. |
| Final conclusion | Authorized human action | terminal diagnosis metadata and close event payload | A conclusion becomes final only after explicit human confirmation or room closure. Automated report delivery, schedule firing, or notification success never finalizes a conclusion. |

## Automated Report Boundary

`FinalReport` is the final artifact of the automated report workflow, not the
final accountable incident conclusion. The name is scoped to the report
workflow fan-in: it means all selected `SubReport` artifacts were reduced,
validated, and persisted for the selected evidence set.

An automated report may:

- summarize the evidence snapshot
- state confidence and uncertainty
- list missing evidence that would improve confidence
- recommend investigation or remediation actions
- trigger a notification that a report is ready for review

An automated report must not:

- claim that an incident is fully resolved
- claim that a root cause is final when confidence or evidence is insufficient
- mutate the immutable evidence snapshot
- execute or mark important operational actions as complete
- replace an explicit human close or confirmation step

## Diagnosis and Supplemental Evidence

Diagnosis room interactions are the supported path for improving confidence
after the first automated report. The AI may answer why confidence is low and
may ask the operator to collect specific data, such as missing metrics, recent
changes, owner confirmation, or logs. When an operator supplies new
information, OpenClarion should retain that information as session or task
context and keep it distinct from the original `EvidenceSnapshot`.

If supplemental context changes the outcome, the final conclusion artifact must
record enough provenance to explain the decision: original snapshot ID,
supplemental context references, confirming operator, timestamp, confidence,
and conclusion version.

## Temporal and Frontend Constraints

Temporal workflow code must stay deterministic. External I/O, LLM calls,
notification sends, database writes, and provider reads belong in Activities
or backend use cases with explicit inputs, timeouts, retries, and idempotency
keys. Workflows should pass immutable snapshot refs, policy IDs, scenario,
correlation keys, and notification channel profile IDs rather than reading
mutable operations configuration directly during replay.

The frontend may present report review, diagnosis, and confirmation workflows,
but it must not make browser state authoritative for final conclusions,
provider credentials, backend addresses, schedule timers, or workflow routing.
Same-origin route handlers may proxy backend actions, validate browser
requests, and keep deployment-specific values on the server side.

## UI Language Rules

Frontend and documentation language should distinguish these commands:

- **Generate report**: starts or replays the automated report workflow.
- **Report ready**: notification state after a persisted report artifact exists.
- **Open diagnosis**: starts human review from frozen evidence.
- **Confirm conclusion**: explicit human action that closes or records the final
  outcome.

Avoid using "final conclusion" for an automated `FinalReport` unless an
authorized human confirmation artifact also exists.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-06-06 | jindyzhao | Added alert report lifecycle boundary between automated report artifacts and human-confirmed final conclusions |
