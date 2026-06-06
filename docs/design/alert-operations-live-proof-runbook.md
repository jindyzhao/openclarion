# Alert Operations Live Proof Runbook

> Last updated: 2026-06-06
> Status: M3.1 operator runbook
> Decision: [ADR-0014](../adr/ADR-0014-alert-operations-configuration.md)

This runbook describes how an operator moves from frontend configuration to
retained live proof for alert operations. It is intentionally alert-first:
Prometheus and Alertmanager are alert source adapters that feed the same
`AlertEvent`, grouping, report, notification, and diagnosis pipeline.

The runbook does not replace the proof validators. It records the expected
configuration order, the frontend/backend authority split, and the external
values that must come from the operator environment before live proof can run.

## Best-Practice Anchors

The configuration path follows the current upstream API semantics:

- Prometheus connection tests use `GET /api/v1/alerts`, which returns the
  standard Prometheus JSON envelope and active alert entries. Upstream
  documentation notes that this endpoint is newer and has weaker stability
  guarantees than the broader API v1 surface, so OpenClarion treats it as an
  adapter-specific connection-test capability rather than a broad provider
  compatibility claim.
- Alertmanager connection tests use `GET /api/v2/alerts` with active-alert
  filtering. Alertmanager webhook ingestion uses the version 4 grouped webhook
  payload shape with top-level group metadata and per-alert labels,
  annotations, timestamps, status, generator URL, and fingerprint.
- Scheduled report triggers use Temporal Schedules through the Go SDK schedule
  client. Registration must use `ScheduleWorkflowAction`, explicit skip
  overlap, bounded catch-up, and runtime describe/reconciliation checks.

## Configuration Object Graph

The frontend settings flow configures a graph of persisted profiles and
policies. The graph is declarative until an operator chooses an explicit action.

```text
AlertSourceProfile
        |
        +--> connection test action
        |
        v
ReportWorkflowPolicy ----> NotificationChannelProfile
        |
        +--> impact preview action
        +--> explicit replay action
        |
        v
ReportWorkflowSchedule
        |
        +--> Temporal Schedule reconciliation

GroupingPolicy
        |
        +--> grouping preview action
        |
        +--> bound by ReportWorkflowPolicy
```

Saves create or replace metadata. They do not start workflows, call alert
providers, resolve secrets, send notifications, create groups, build snapshots,
or own timers. Actions are named separately so operators can test, preview,
enable, replay, and prove the configuration without making form persistence a
hidden trigger.

## Frontend Configuration Order

Operators should configure the browser-visible state in this order:

1. **Settings overview** at `/settings`.
   Review the declarative configuration graph and current profile counts before
   entering the individual configuration surfaces. The overview reads
   server-owned state only. After the graph contains at least one object for
   each required profile/policy/schedule type, the overview separates the
   retained proof handoff into policy replay and scheduled-trigger targets.
   These cards are display-only readiness projections; they do not persist
   drafts, start workflows, call providers, resolve secrets, schedule timers, or
   prove acceptance by themselves.
2. **Alert source profile** at `/settings/alert-sources`.
   Create a Prometheus or Alertmanager profile with display name, source kind,
   base URL, auth mode, optional `secret_ref`, enabled state, and labels. The
   browser submits metadata only; it never stores bearer values.
3. **Connection test** from the alert source row action.
   The backend performs bounded provider I/O and returns sanitized status,
   reason, checked time, kind, auth mode, and small alert counters. A passing
   test does not enable any workflow policy by itself.
4. **Grouping policy** at `/settings/grouping-policies`.
   Configure grouping dimension label keys, severity label key, optional source
   filter, and enabled state. Preview runs only against bounded persisted
   `AlertEvent` samples.
5. **Notification channel profile** at `/settings/notification-channels`.
   Configure display name, adapter kind, delivery scopes, enabled state,
   labels, and `secret_ref`. The browser must not collect endpoint URLs or
   credentials. The row-level test action sends one controlled backend test for
   the persisted channel ID and returns sanitized feedback.
6. **Report workflow policy** at `/settings/report-workflow-policies`.
   Bind the enabled alert source, enabled grouping policy, report scenario,
   diagnosis follow-up mode, and optional report-capable notification channel.
   Saves are metadata changes. Enable and disable are separate backend actions.
7. **Impact preview** from the report workflow policy row action.
   The backend reviews binding readiness and recent persisted alert samples
   without provider I/O, secret resolution, workflow starts, or notification
   sends.
8. **Explicit policy replay** from the report workflow policy row action.
   The browser sends only policy ID, replay window, limit, and optional
   idempotency identifiers. Backend code resolves the stored policy, alert
   source, grouping policy, scenario, and server-side credentials before
   starting report generation.
9. **Report workflow schedule** at `/settings/report-workflow-schedules`.
   Configure persisted schedule metadata for one report workflow policy:
   Temporal Schedule ID, interval and offset, replay window and delay, replay
   limit, and catch-up window. The browser does not own timers, cron state,
   direct Temporal calls, provider calls, secret resolution, workflow starts, or
   notification sends.

## Rule and Workflow Model

Grouping rules and report workflows are configured as separate objects because
they change at different rates and have different side effects.

| Object | Operator intent | Side-effect boundary |
|--------|-----------------|----------------------|
| Alert source profile | Identify an alert adapter and credentials reference | Connection test only |
| Grouping policy | Define labels, severity key, and optional source scope | Preview over persisted samples only |
| Notification channel profile | Identify delivery target metadata and secret reference | Channel test only |
| Report workflow policy | Bind source, grouping, scenario, follow-up, and delivery | Enable, preview, or replay actions |
| Report workflow schedule | Bind a policy to a recurring replay window | Server-owned Temporal reconciliation |

The report workflow policy is the operator-owned workflow contract. It owns the
report scenario and binding IDs; replay requests only provide the policy ID,
window, limit, and optional idempotency identifiers. This prevents browser
forms from overriding workflow routing or report behavior at action time.

The `/settings` overview projects the same model for handoff. Its policy replay
target points operators to the manual policy-replay proof chain: PostgreSQL,
Temporal, alert source, worker LLM, and notification delivery must all be real
operator-provided services before retained evidence can pass. Its scheduled
trigger target points to the separate schedule proof chain: an enabled persisted
schedule, a real Temporal Schedule action, launcher workflow execution, report
delivery, and retained validator output. These targets are intentionally
separate because a successful policy replay does not prove schedule firing, and
a schedule action without downstream report delivery does not prove acceptance.

## Alertmanager Webhook Intake

Alertmanager webhook intake is an ingestion trigger, not a report workflow
trigger. Configure an enabled Alertmanager alert source profile first, then
configure Alertmanager to send receiver webhooks to:

```text
/api/v1/alert-sources/{source_id}/webhooks/alertmanager
```

The endpoint accepts version 4 grouped webhook payloads, persists firing alerts
as `AlertEvent` rows, skips resolved alerts in the current slice, and returns
bounded counters. It does not start report workflows, build snapshots, resolve
notification channels, or send notifications.

## External Inputs Required for Proof

The following values must come from the operator environment. They must not be
committed to source, retained GitHub comments, fixtures, or browser durable
state.

| Area | Required input |
|------|----------------|
| Database | `DATABASE_URL` for the PostgreSQL instance containing the configured profiles and alert data |
| Temporal | `TEMPORAL_HOST_PORT` for the Temporal service used by the worker and proof command |
| Replay window | `REPORT_WINDOW_START` and `REPORT_WINDOW_END` as canonical UTC timestamps |
| Policy proof | `REPORT_WORKFLOW_POLICY_ID` for an enabled report workflow policy |
| Schedule proof | `REPORT_WORKFLOW_SCHEDULE_ID` and `REPORT_WORKFLOW_POLICY_ID` for an enabled schedule/policy binding |
| Output retention | `REPORT_POLICY_LIVE_SMOKE_OUTPUT` or `REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT` pointing at a new local JSON proof path |
| Worker LLM | `OPENCLARION_LLM_MODEL` plus the deployment's `OPENCLARION_LLM_*` provider settings, unless an externally managed worker is already verified |
| Alert source secrets | `OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON` when configured profiles need bearer tokens |
| Notification delivery | `OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON` for profile-bound delivery, or `OPENCLARION_IM_WEBHOOK_URL` for legacy unbound delivery |
| Worker assertion | `REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1` only after the operator has verified the worker already has the required LLM and notification wiring |

## Proof Commands

Run readiness checks before running live proof:

```bash
export REPORT_POLICY_LIVE_SMOKE_OUTPUT=/path/to/new-policy-proof.json
export REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT=/path/to/new-schedule-proof.json
make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=report-policy-live-smoke
make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=report-schedule-live-smoke
make report-policy-live-smoke
make report-schedule-live-smoke
```

`make report-policy-live-smoke` runs the profile-driven replay path and writes
validator-checked JSON with `request.policy_id` to the explicit
`REPORT_POLICY_LIVE_SMOKE_OUTPUT` path. Readiness rejects a missing or already
existing output path without printing it, and the proof script fails before
writing when that readiness target is blocked. The command proves the enabled
report-workflow-policy path only when it runs against real database, Temporal,
alert source, worker, LLM, and notification delivery configuration.

`make report-schedule-live-smoke` runs the scheduled-trigger proof harness. It
first runs the readiness preflight, then loads the persisted enabled schedule,
waits for a Temporal Schedule action at or after
`REPORT_SCHEDULE_OBSERVED_AFTER`, waits for the launcher workflow and the
downstream `ReportBatchWorkflow`, and writes validator-checked JSON linking the
schedule configuration, launcher workflow, report batch workflow, final report,
and notification delivery. The script fails before writing when scheduled
readiness is blocked. The command only proves scheduled delivery when it runs
against real database, Temporal, alert source, worker, LLM, and
notification delivery configuration with retained output.

## Non-Goals

- Do not store real endpoint URLs, bearer tokens, or secret values in browser
  durable state.
- Do not treat a successful connection test, grouping preview, channel test, or
  impact preview as workflow enablement.
- Do not claim live external proof from readiness output alone.
- Do not put customer endpoint values, credentials, local proof paths, or raw
  alert payloads into public GitHub issue or pull-request text.
