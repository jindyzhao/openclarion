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
| `ReportWorkflowPolicy` | trigger mode, report scenario, alert source binding, grouping policy binding, optional report notification channel binding, explicit enablement state, and diagnosis-room follow-up behavior |
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

The operator-facing configuration sequence is documented in
[alert-operations-live-proof-runbook.md](../design/alert-operations-live-proof-runbook.md).
The sequence is alert source profile -> connection test -> grouping policy ->
notification channel profile -> report workflow policy -> impact preview ->
explicit replay -> optional report workflow schedule. The frontend may guide
operators through that sequence, but each step remains a server-owned
configuration or action boundary; the browser never becomes the durable source
of provider credentials, workflow routing, schedule timers, or notification
delivery state.

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

Report workflow policies use a dedicated profile and explicit enable/disable
actions. The current slice stores a disabled draft with an alert source profile
ID, grouping policy ID, optional report notification channel profile ID, manual
replay trigger mode, report scenario, and diagnosis-room follow-up mode.
Enabling a policy validates that the bound alert source profile and grouping
policy both exist and are already enabled. If a report notification channel is
bound, enablement also validates that the channel exists, is enabled, and
declares the report delivery scope. Creating, updating, enabling, or disabling
a policy must not start Temporal workflows, cancel Temporal workflows, call
Prometheus or Alertmanager, send notifications, or replace the existing
environment-variable live-smoke path.

Policy-driven report replay is a separate explicit action. The action resolves
the enabled `ReportWorkflowPolicy`, its enabled `AlertSourceProfile`, and its
enabled `GroupingPolicy` before any workflow start. Provider construction and
server-side secret resolution happen in backend code after the configuration
read, and the browser sends only the policy ID, replay window, limit, and
optional idempotency/workflow identifiers. The stored policy owns the report
scenario; the frontend does not override scenario at replay time. The replay
path applies the bound grouping dimensions, severity key, and source filter
before starting the report batch workflow.

Report workflow output follows the lifecycle boundary in
[report-lifecycle.md](../design/report-lifecycle.md). A persisted
`FinalReport` is the final artifact of the automated report workflow, not the
final accountable incident conclusion. Policy replay, schedule firing, report
delivery, and notification success may prove that the automated report path
worked, but they must not claim human-confirmed closure. Confidence can improve
only through additional retained evidence or diagnosis-room context, and final
conclusions require an explicit authorized confirmation or close artifact.

Alertmanager webhook intake is a separate alert-ingestion trigger, not a
report workflow trigger. The endpoint is bound to a stored
`AlertSourceProfile` whose kind is `alertmanager` and whose profile is
enabled. It accepts Alertmanager webhook receiver payloads, parses only the
documented grouped alert fields, persists firing alerts through the same
`AlertEvent` ingestion boundary used by provider polling, and returns bounded
ingest counters. It must use strict, size-limited JSON decoding; must not echo
raw alert payloads, upstream URLs, bearer values, or secret references in the
response; and must not call Prometheus or Alertmanager, resolve workflow
policies, start Temporal workflows, create `AlertGroup` /
`EvidenceSnapshot` rows, resolve notification channels, or send
notifications. Resolved webhook alerts are intentionally skipped in the first
slice until alert resolution update semantics have a dedicated implementation
and tests. When the bound profile uses bearer authentication, inbound
webhook authorization is checked against the deployment-managed secret
resolver before ingestion.

Scheduled report triggering is a later extension of the same persisted policy
model, not a second frontend-owned workflow system. A schedule must be stored
as server-side configuration bound to one `ReportWorkflowPolicy`, with an
explicit enable/disable action separate from profile edits. The scheduler must
use Temporal Schedules rather than an in-process cron loop. OpenClarion must
reconcile persisted schedule rows into Temporal Schedules at runtime and after
successful schedule mutations; the browser never owns timers, cron state, or
Temporal calls. The scheduled action starts a small launcher workflow with
immutable inputs: policy ID, windowing policy, replay limit, and schedule fire
time. That launcher may call Activities to resolve the current enabled
policy/source/grouping/channel binding and replay the alert window, then start
the existing report batch workflow from immutable EvidenceSnapshot refs. The
report batch workflow must still receive only immutable snapshot refs,
scenario, correlation key, and notification channel profile ID; it must not
read mutable operations configuration directly.

Scheduled overlap handling must be fail-closed. The default schedule overlap
policy is skip, so a slow replay does not create concurrent report batches for
the same policy. Catch-up behavior must be bounded to avoid backfilling an
unreviewed outage window by surprise. Saving or replacing a schedule must not
start a workflow, call Prometheus or Alertmanager, resolve secrets, or send
notifications. It may create or update the corresponding Temporal Schedule so
server-owned schedule state remains convergent with PostgreSQL. Disabling a
schedule pauses future starts only; it does not cancel already-started report
workflows.

Scheduled-trigger live proof must stay separate from configuration readiness.
`make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=report-schedule-live-smoke`
confirms that the operator has supplied the local schedule, policy binding,
Temporal, worker, secret-resolver, and retained-output prerequisites without
printing their values. That readiness output does not prove scheduled report
delivery. `make report-schedule-live-smoke` is the proof harness: it loads the
persisted enabled schedule, waits for a Temporal Schedule action at or after the
operator-selected observation time, waits for the launcher workflow and the
downstream report batch workflow, and validates retained JSON that ties the
schedule configuration to the resulting report workflow and notification
delivery. Closing the scheduled-trigger proof item still requires running that
harness against real services and retaining the proof artifact.

Report workflow impact preview is also a separate explicit action. The action
loads the persisted report workflow policy, its bound alert source profile, its
bound grouping policy, and, when present, its report notification channel
profile. It then reads a bounded recent `AlertEvent` sample from PostgreSQL and
applies the same deterministic grouping preview used by grouping policies. The
action returns readiness status, stable reason codes, sampled event/group
counters, and bounded group samples. It must not call Prometheus or
Alertmanager, resolve secrets, construct metrics or notification providers,
start Temporal workflows, send notifications, persist `AlertGroup` or
`EvidenceSnapshot` rows, or treat a ready preview as enablement.

Notification channel profiles use a dedicated profile for delivery target
metadata. A profile stores name, adapter kind, `secret_ref`, delivery scopes,
enabled state, and labels. Create, replace, list, and get operations must not
store endpoint URLs, resolve secrets, construct a Webhook IM provider, send
notifications, or replace the existing environment-variable
`OPENCLARION_IM_WEBHOOK_URL` runtime path. Report workflow policies may bind an
optional report notification channel profile ID as configuration metadata.
Report notification Activities may resolve that bound ID through a configured
backend secret resolver and construct the Webhook IM provider at runtime. If no
profile is bound, the existing `OPENCLARION_IM_WEBHOOK_URL` path remains the
fallback for legacy or unbound report notifications. Workflow code carries only
the immutable profile ID and never resolves secrets or providers.

The operations configuration hygiene gate is part of this boundary. It scans
the alert-operations configuration surface for non-placeholder HTTP(S) hosts,
URL credentials outside test fixtures, URL query/fragment leakage outside test
fixtures, and browser durable storage APIs in `web/src`. This complements
gitleaks: gitleaks detects secret-shaped values across history and current
source, while the hygiene gate enforces the product-specific rule that
operator configuration remains server-owned and does not persist customer
endpoints, bearer tokens, or secret values in frontend durable state.

### Workflow Boundary

Workflow starts receive immutable request metadata after backend code resolves
policy, provider, grouping, and credential state. Activities may load provider
profiles by ID when performing external I/O, but Temporal workflow code must
not call providers directly and must not depend on live configuration mutation
for deterministic replay.

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
* report workflow policies can be stored and explicitly enabled with validated
  alert source, grouping policy, and optional report notification channel
  identifiers, and an explicit replay action can start report generation from
  those bindings while preserving the existing Prometheus live-smoke path until
  `make report-policy-live-smoke` captures equivalent retained evidence with
  `request.policy_id`
* generated report artifacts remain distinct from final accountable
  conclusions, and operator-facing flows do not treat replay, schedule firing,
  report persistence, or notification delivery as human-confirmed closure
* Alertmanager webhook payloads can be ingested into `AlertEvent` rows through
  an enabled Alertmanager alert-source profile without starting report
  workflows or echoing raw alert content
* scheduled report triggers can be stored, paused, and resumed through
  server-owned configuration, and their Temporal Schedule actions use skip
  overlap plus immutable launcher-workflow inputs instead of browser-owned cron
  state or dynamic configuration reads inside report workflows; the
  scheduled-trigger proof harness is available, while retained live evidence
  remains a separate operator-run proof against real services
* report workflow impact previews can review binding readiness and bounded
  recent-alert grouping impact without provider I/O, workflow starts, or
  durable grouping/snapshot writes
* notification channel profiles can be stored, listed, updated, and disabled
  while storing only `secret_ref` values and never returning endpoint URLs or
  credential values, and report notification Activities can use a configured
  backend resolver to deliver through an enabled report-scoped bound profile
* frontend settings pages use generated OpenAPI types and do not persist real
  bearer tokens or URLs outside user-submitted API calls
* `make operations-config-hygiene` passes, proving the alert-operations
  configuration surface does not hard-code customer endpoints or use browser
  durable storage for operator configuration

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
has equivalent retained evidence. `make report-policy-live-smoke` is the
manual proof boundary for that replacement: it must run the persisted policy
path and retain validator-checked JSON with `request.policy_id` before the
project claims profile-driven live acceptance. Alertmanager support should land
as a separate provider adapter with fake or `httptest` coverage and contract
tests, not as Prometheus-specific branching in frontend code. Secret-backed
connectivity tests and policy-driven replay require an explicit backend
resolver map; they must not let operator-submitted `secret_ref` values read
arbitrary process environment variables, and they must not expose raw secret
values to OpenAPI responses, logs, or the browser.

Temporal Go SDK v1.44.0 exposes Schedule creation through
`client.ScheduleClient().Create` with `client.ScheduleOptions`, a
`client.ScheduleSpec`, a `client.ScheduleWorkflowAction`, and an `Overlap`
field. OpenClarion should use interval or calendar specs for new schedules and
set overlap explicitly to `SCHEDULE_OVERLAP_POLICY_SKIP` when registering
report-policy schedules.

The scheduled-trigger persistence and settings slices store
`ReportWorkflowSchedule` metadata in PostgreSQL, expose generated API contracts
and a frontend settings surface, and validate explicit enablement against an
already-enabled `ReportWorkflowPolicy`. The launcher and registration-builder
slice adds `ReportPolicyScheduleLauncherWorkflow`,
`RunScheduledReportPolicyReplay`, worker-side policy replayer injection, and
`ReportWorkflowScheduleRegistrar` mapping from persisted schedule metadata to
Temporal `ScheduleOptions`. The runtime reconciliation slice creates missing
Temporal Schedules, updates existing schedule specs/actions/policies, and
synchronizes paused state from persisted enablement during startup and after
successful schedule create, replace, enable, or disable actions. The
scheduled-trigger proof harness waits for a real Schedule action, waits the
launcher and downstream report batch workflows, and validates retained
report-delivery JSON. Retained live proof against an external Prometheus or
Alertmanager scheduled run remains pending until an operator runs the harness
against real services.

Alertmanager webhook receiver payloads follow the upstream version 4 grouped
JSON shape documented by Alertmanager: top-level group metadata plus an
`alerts` array whose entries include status, labels, annotations, `startsAt`,
`endsAt`, `generatorURL`, and `fingerprint`. OpenClarion should parse that
shape as an adapter boundary and keep the returned API response limited to
ingest counters.

The live proof runbook records the external values operators must provide
before `make report-policy-live-smoke` or scheduled-trigger proof can support
acceptance. Those values include real PostgreSQL and Temporal addresses,
canonical UTC replay windows, enabled policy or schedule identifiers,
report-capable worker configuration, secret-reference resolver maps when
bearer-backed alert sources or profile-bound notification channels are used,
and a new retained JSON output path. None of those values belong in source
files, browser durable state, fixtures, retained public comments, or generated
examples.

The report lifecycle boundary records the product wording that implementation
and frontend surfaces must preserve: automated report generation produces
reviewable report artifacts with confidence and missing-evidence guidance,
while final incident conclusions are recorded only through explicit human
confirmation or diagnosis-room closure.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-06-05 | jindyzhao | Initial proposal |
| 2026-06-05 | jindyzhao | Added sanitized alert-source connection-test boundary |
| 2026-06-05 | jindyzhao | Added grouping policy persistence and dry-run preview boundary |
| 2026-06-05 | jindyzhao | Added server-side secret resolver and Alertmanager connection-test adapter boundary |
| 2026-06-05 | jindyzhao | Added report workflow policy persistence and explicit enablement boundary |
| 2026-06-05 | jindyzhao | Added notification channel profile persistence and secret-ref-only frontend boundary |
| 2026-06-05 | jindyzhao | Added explicit policy-driven report replay action boundary |
| 2026-06-05 | jindyzhao | Added operations configuration hygiene gate boundary |
| 2026-06-05 | jindyzhao | Added optional report workflow policy to notification channel binding boundary |
| 2026-06-05 | jindyzhao | Added profile-backed report notification Activity delivery boundary |
| 2026-06-05 | jindyzhao | Added report workflow impact preview boundary |
| 2026-06-05 | jindyzhao | Added profile-driven report workflow live-smoke proof boundary |
| 2026-06-05 | jindyzhao | Added scheduled report trigger boundary using Temporal Schedules with skip overlap and immutable launcher inputs |
| 2026-06-05 | jindyzhao | Added scheduled-trigger persistence slice boundary before Temporal registration |
| 2026-06-06 | jindyzhao | Added scheduled-trigger generated API and frontend settings boundary before launcher workflow and Temporal registration |
| 2026-06-06 | jindyzhao | Added scheduled-trigger launcher workflow, replay Activity wiring, worker policy-replayer injection, and Temporal ScheduleOptions registration builder while leaving runtime reconciliation and retained live proof pending |
| 2026-06-06 | jindyzhao | Added runtime scheduled-trigger reconciliation boundary for startup and post-mutation Temporal Schedule synchronization while leaving retained live proof pending |
| 2026-06-06 | jindyzhao | Added Alertmanager webhook intake boundary for profile-bound alert ingestion without workflow starts |
| 2026-06-06 | jindyzhao | Added operator runbook projection for alert operations configuration and retained live-proof prerequisites |
| 2026-06-06 | jindyzhao | Added report lifecycle boundary that separates automated report artifacts from human-confirmed final conclusions |
| 2026-06-06 | jindyzhao | Added scheduled-trigger proof harness boundary for real Temporal Schedule action to report delivery verification |
