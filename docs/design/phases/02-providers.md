# Phase 02: Providers

## Goal

Implement default and fake providers for local development and workflow tests.

## Default Providers

| Provider | Default target |
|----------|----------------|
| MetricsProvider | Prometheus alerts |
| AlertSource adapter | Alertmanager contract placeholder |
| CMDBProvider | static YAML and generic HTTP adapter |
| IMProvider | Email, Webhook, Slack |
| AuthProvider | local development auth, OIDC later |
| ApprovalProvider | no-op and manual approval |
| ContainerProvider | Docker local |
| LLMProvider | mock and OpenAI-compatible endpoint |

## Operator Integration Profiles

Provider interfaces remain compile-time Go contracts. Operator-facing
configuration is separate product state, described by
[ADR-0014](../../adr/ADR-0014-alert-operations-configuration.md).
Provider capability and dependency boundaries follow
[ADR-0003](../../adr/ADR-0003-provider-extension-interfaces.md).

The provider phase now has two responsibilities:

- keep concrete provider adapters small and testable with fakes
- expose alert source connection metadata through backend-owned profiles rather
  than hard-coded frontend constants or customer endpoints in tests

Prometheus is the first implemented alert source adapter. Alertmanager must
land as its own adapter with contract tests and fake coverage before product
documentation or UI can claim Alertmanager support. Both adapters must translate
upstream payloads into the same `ActiveAlert` / `AlertEvent` path.

## Generic HTTP CMDB Runtime Contract

CMDB enrichment is disabled when all three runtime variables are absent. Set
`OPENCLARION_CMDB_HTTP_URL` to enable it, with optional
`OPENCLARION_CMDB_HTTP_BEARER_TOKEN` and a positive integer
`OPENCLARION_CMDB_HTTP_TIMEOUT_SECONDS` (default: 10). A token or timeout without
the URL is rejected at process startup.

The configured endpoint receives one context-bound `POST` per grouped alert
event. Requests use `application/json`; bearer authentication is added only
when configured:

```json
{
  "labels": {
    "alertname": "HighErrorRate",
    "service": "payments"
  }
}
```

A normal no-match response must omit `resource`:

```json
{
  "found": false
}
```

A match returns one sanitized provider-neutral resource projection:

```json
{
  "found": true,
  "resource": {
    "id": "service:payments",
    "kind": "service",
    "name": "payments",
    "owners": [
      {
        "subject": "team-payments",
        "team": "Payments",
        "role": "primary"
      }
    ],
    "topology": [
      {
        "relation": "depends_on",
        "target_id": "database:ledger",
        "target_kind": "database",
        "target_name": "ledger"
      }
    ],
    "attributes": {
      "tier": "critical"
    }
  }
}
```

Unknown fields, duplicate JSON keys, trailing JSON values, oversized responses,
invalid resource projections, non-2xx statuses, and request failures are
rejected. Redirects are not followed, and transport errors do not expose the
configured request URL. A lookup failure fails only the affected replay group;
provider I/O does not run inside the group write transaction.

## Provider Rules

- Provider implementations must not leak credentials through logs.
- Provider errors must be wrapped with context and mapped to safe user-facing
  messages at API boundaries.
- Provider tests must not require production systems.
- Provider examples must use reserved or local example hosts only.
- APIs may store and return `secret_ref` values, but must never return secret
  material.

## Acceptance

- fake providers cover workflow tests
- default providers support local MVP replay
- provider interfaces are stable enough for P0/P1
- alert source profiles can be managed without rebuilding the control plane
- profile connectivity tests return sanitized results and do not enable a
  source implicitly
