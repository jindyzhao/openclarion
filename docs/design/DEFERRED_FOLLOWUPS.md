# Deferred Follow-ups

> Tracks decisions that have been intentionally deferred. Each entry records
> what was deferred, why, and the trigger that should bring it back. This
> prevents deferred items from being silently lost and prevents past
> discussions from being re-litigated without new information.

> Last updated: 2026-05-19

## How To Use This File

* Add an entry when a decision is made to defer something.
* Each entry must specify a re-evaluation trigger or a target milestone.
* Remove an entry only when the work is shipped (and recorded in
  [CURRENT_STATE.md](CURRENT_STATE.md)) or the decision is reaffirmed in a
  superseding ADR.

## Active Deferrals

### D1: Workflow Engine Spike (Temporal vs River+sqlc)

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | Temporal selection in ADR-0004 is driven by M5 short-conversation diagnosis. A 1-week spike comparing Temporal and River+sqlc on the M2 fan-out/fan-in path would cross-check the choice. Spike not run because pre-M0 has no code yet. |
| Trigger | run only if M0 Temporal setup or M2 fan-out/fan-in implementation feels disproportionate |
| Target | optional M0/M1 cross-check |

### D2: oapi-codegen-exp StrictServerInterface Adapter

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | V3 of `oapi-codegen-exp` does not currently generate `StrictServerInterface`. V1 will accept the non-strict `ServerInterface`. A thin typed adapter layer can be written if request/response wrapping becomes a maintainability problem. |
| Trigger | revisit if generated handlers accumulate boilerplate that strict typing would eliminate, or if upstream adds Strict support |
| Target | M2 (only if pain materializes) |

### D3: Lifecycle-End Conversation Compression (M5)

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | M5 V1 scope is short-conversation only. Bounded turns and fixed lifetime cap session size, so automatic compression is not needed for V1. |
| Trigger | revisit when long-session product validation justifies multi-day rooms |
| Target | post-V1 |

### D4: Leader-Tier RBAC and Approval Flows (M5)

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | Owner + admin RBAC suffices for V1 short-conversation diagnosis. Multi-stakeholder approval adds significant complexity without proportional V1 value. |
| Trigger | post-V1 product feedback identifies multi-approver scenarios |
| Target | post-V1 |

### D5: Streaming Token-Level Partial Responses (M5)

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | Turn-by-turn delivery is sufficient for short-conversation diagnosis. Streaming partial tokens adds significant complexity to the WebSocket proxy and workflow contract. |
| Trigger | UX validation after V1 ships |
| Target | post-V1 |

### D6: Concrete Version Pinning for Temporal SDK / Ent / Atlas / OTel

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | DEPENDENCIES.md still uses `TBD pinned at M0` for several entries. M0 must replace these with concrete `module@version` and document any compat caveats. |
| Trigger | M0 PR introduces `go.mod` with concrete versions |
| Target | M0 |

### D7: pgvector / RAG Retrieval

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | RAG is not part of MVP. Reports work without retrieval. Adding pgvector early would expand schema and ops surface for unproven value. |
| Trigger | post-V1 product validation indicates report quality is bottlenecked by missing context |
| Target | post-V1 |

### D8: Kubernetes Job ContainerProvider

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | M4 Docker sandbox is sufficient for control-plane-owned analysis. Kubernetes Job runtime is a future ContainerProvider implementation behind the same interface. |
| Trigger | deployment requirements demand orchestrated multi-tenant sandboxes |
| Target | post-V1 |

### D9: Email and Slack IMProvider Implementations

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | M2 ships Webhook IMProvider only. Email and Slack are valuable but not on the V1 critical path. |
| Trigger | first deployment with operational notification preferences |
| Target | post-V1 |

### D10: Phase Checklist File Split

| Field | Value |
|-------|-------|
| Status | open |
| Decided | 2026-05-19 |
| Reason | Single `CHECKLIST.md` covers all milestones. Splitting into per-phase files (`checklist/phase-*.md`) is a maintenance choice, not a functional improvement, while the project remains a single maintainer. |
| Trigger | re-evaluate after M2 if checklist length harms readability |
| Target | M2 review |

## Closed Deferrals

(none yet)

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-19 | jindyzhao | Initial set of deferrals: spike, Strict adapter, M5 long-session features, version pinning, future providers |
