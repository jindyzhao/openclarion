# Alert-First Signal Extension Model

OpenClarion remains an intelligent alert analysis system. Its first supported
and validated product path is IT and operations alert analysis: ingest alert
state, group related alerts, freeze evidence, generate structured reports,
notify humans, and retain an audit trail.

The architecture should still avoid hard binding the control plane to one
monitoring product or one alert domain. The reusable abstraction is an
alert-first signal workflow: a triggering event, deterministic grouping,
evidence capture, AI-assisted reporting, durable orchestration, and human
decision making.

This document is not a product repositioning. It is an extension boundary for
engineering decisions that would otherwise overfit the framework to one
monitoring backend or one alert taxonomy.

## Boundary

| Current implementation term | General design role | Current priority |
|-----------------------------|---------------------|------------------|
| `AlertEvent` | Triggering signal event | Keep as the public and storage name for the alert-analysis product path |
| `AlertGroup` | Deterministic case or signal group | Keep as the public and storage name for grouped alerts |
| `EvidenceSnapshot` | Frozen evidence package | Shared across alert and future signal domains |
| `SubReport` / `FinalReport` | Structured AI report output | Shared report lifecycle with scenario-specific schemas over time |
| Temporal workflows | Retry, timeout, idempotency, and long-running state | Control-plane baseline |
| Provider interfaces | External system boundaries | Metrics and alert providers first; additional providers later |

`SignalEvent`, `SignalGroup`, or `CaseGroup` are useful architecture aliases,
not immediate rename targets. A rename would affect OpenAPI contracts,
persistence tables, migrations, dashboards, documentation, and operator mental
models. It should only happen after a compatibility plan exists.

## Design Rule

New framework code should be alert-first and signal-capable:

- Use alert terminology for current product behavior and public workflows.
- Keep provider ports small and capability-based, so future sources can be
  added without changing alert orchestration.
- Freeze all AI input in `EvidenceSnapshot`; AI output must be traceable to
  that snapshot and schema-validated before persistence.
- Keep workflow decisions in Go and Temporal. LLMs and sandboxed agents analyze
  prepared evidence but do not own routing, lifecycle, persistence, or final
  decisions.
- Treat humans as the accountable decision makers for business-impacting
  outcomes.

This aligns with Temporal workflow practice: deterministic workflow code keeps
orchestration state, while Activities handle side effects and external API
calls with explicit timeouts, retry policies, and idempotent writes.

## Future Provider Examples

The current provider set stays focused on alerts and follows the capability
boundary in
[ADR-0003](../adr/ADR-0003-provider-extension-interfaces.md):

- `ActiveAlertProvider` as the minimum alert-source capability
- optional `MetricQueryProvider` for Prometheus-compatible metric evidence
- alert source adapters such as Prometheus and Alertmanager
- `CMDBProvider`
- `LLMProvider`
- `IMProvider`
- `ContainerProvider`
- `AuthProvider`
- `ApprovalProvider`

Future business-signal use cases may add provider ports such as:

- `ClaimProvider`
- `PolicyProvider`
- `DocumentProvider`
- `FraudProvider`
- `WeatherProvider`
- `GISProvider`
- `CustomerServiceProvider`
- channel or vendor data providers

Those future providers should feed the same evidence and reporting pipeline
instead of bypassing it.

## Optional Future Domain Examples

For a property and casualty insurer, the same skeleton could support these
future scenarios without changing the current alert-analysis priority:

| Scenario | How it maps | Risk posture |
|----------|-------------|--------------|
| Claims triage | claim registration, loss amount, line of business, region, customer tier, and missing documents become grouped signals | human adjuster remains accountable |
| Fraud or SIU lead governance | fraud model hits, repeated claims, unusual repair shops, or abnormal payment paths become grouped evidence | AI summarizes leads; it does not sanction |
| Catastrophe response | weather events, claim spikes, geospatial exposure, and resource constraints become regional evidence groups | AI builds situation reports |
| Complaint or regulatory risk | complaints, delays, dispute events, service ratings, and regulator referrals become timeline evidence | AI highlights risk points |
| Underwriting exception review | high-risk submissions, missing information, prior claims, and exposure signals become review prompts | AI assists underwriting questions |
| Claims SLA or leakage monitoring | overdue cases, authority overrides, repeated adjustments, and closure-cycle anomalies become operations signals | AI supports operational governance |
| Vendor risk review | cycle time, amount deviation, rework rate, complaints, and fraud associations are grouped by vendor | AI builds an audit-ready profile |
| Subrogation or salvage opportunity | third-party liability, police records, liability ratios, and salvage status become opportunity signals | AI recommends follow-up actions |

These are optional extension examples, not current MVP scope, roadmap priority,
or public positioning. The current product track remains intelligent alert
analysis.

## Governance Guardrails

Future business-impacting scenarios must remain human-review workflows. Avoid
automatic pricing, automatic denial, automatic rejection, or automatic fraud
penalties unless a later governance package explicitly covers the decision
rights, audit requirements, fairness controls, and regulatory review.

The relevant external governance direction is consistent with this boundary:
the NAIC AI Model Bulletin emphasizes governance, risk management, consumer
impact accountability, documentation, and oversight for insurer AI use; EIOPA's
AI governance opinion highlights data governance, record keeping, fairness,
cybersecurity, explainability, and human oversight.

Sources:

- [NAIC: Members Approve Model Bulletin on Use of AI by Insurers](https://content.naic.org/article/naic-members-approve-model-bulletin-use-ai-insurers)
- [EIOPA: Opinion on AI Governance and Risk Management](https://www.eiopa.europa.eu/eiopa-publishes-opinion-ai-governance-and-risk-management-2025-08-06_en)

## Migration Sequence

1. Document the alias model and provider extension boundary.
2. Keep current `AlertEvent` and `AlertGroup` code names stable through the
   alert-analysis MVP.
3. Add scenario-specific report schemas only when a real use case appears.
4. Add provider ports one capability at a time, with fake implementations and
   contract tests in the same change.
5. Consider public naming changes only after OpenAPI compatibility,
   persistence migration, dashboard copy, and operator documentation are
   planned together.
