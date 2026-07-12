# Phase 02: Providers

## Goal

Implement default and fake providers for local development and workflow tests.

## Default Providers

| Provider | Default target |
|----------|----------------|
| ActiveAlertProvider | Prometheus and Alertmanager active alerts |
| MetricQueryProvider | optional Prometheus-compatible metric queries |
| CMDBProvider | static YAML, generic HTTP, and NetBox 4.5.2+ adapter |
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

Prometheus and Alertmanager translate upstream payloads into the same
`ActiveAlert` / `AlertEvent` path. The source builder uses a validated
kind-to-factory registry: every adapter supplies `ActiveAlertProvider`, while
only Prometheus-compatible adapters supply `MetricQueryProvider`.

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

## NetBox CMDB Runtime Contract

The direct NetBox adapter is mutually exclusive with the generic HTTP adapter.
Set both `OPENCLARION_CMDB_NETBOX_URL` and
`OPENCLARION_CMDB_NETBOX_LOOKUP_LABEL` to enable it. The URL may identify the
deployment root or its `/api` root. Optional settings are:

- `OPENCLARION_CMDB_NETBOX_API_TOKEN`
- `OPENCLARION_CMDB_NETBOX_TOKEN_SCHEME=auto|bearer|token` (default: `auto`)
- `OPENCLARION_CMDB_NETBOX_LOOKUP_FILTER` (default: `name`)
- `OPENCLARION_CMDB_NETBOX_OBJECT_TYPE=auto|device|virtual_machine`
  (default: `auto`)
- `OPENCLARION_CMDB_NETBOX_ATTRIBUTE_CUSTOM_FIELDS` as a comma-separated
  allowlist of at most 32 scalar custom fields
- `OPENCLARION_CMDB_NETBOX_TIMEOUT_SECONDS` as a positive integer
  (default: 10)

In automatic token mode, NetBox v2 tokens beginning with `nbt_` use Bearer
authentication. Other tokens use the legacy `Token` scheme; operators can set
the scheme explicitly during migrations. Anonymous reads remain possible when
the NetBox deployment permits them and no token is configured.

The configured alert label value is sent as one NetBox filter query. A missing
label is a normal no-match and performs no request. Device and virtual-machine
lookups request at most two rows and accept exactly zero or one. `auto` searches
both collections and rejects a cross-type match rather than choosing one
silently. Contact assignments are capped at 32 and map to owners. Site,
location, rack, cluster, and VM host references map to stable topology links.
Standard role, tenant, platform, status, primary IP, and tag fields map to
sanitized `netbox.*` attributes. `custom_fields` is not requested when the
allowlist is empty; when configured, only allowlisted string, number, or boolean
values are retained as `netbox.custom.*` attributes.

The adapter requires NetBox release 4.5.2 or newer within the 4.x series. This
floor is required for the `fields` response projection used to bound and
allowlist mapped data. NetBox normally publishes only major and minor values in
the `API-Version` header, so a present header is checked for a compatible 4.5+
API while unsupported patch releases fail their `fields` requests. The adapter
caps every response at 2 MiB, rejects duplicate JSON keys, trailing JSON values,
malformed pagination, ambiguous results, invalid mapped identifiers, redirects,
and non-2xx statuses. Transport errors omit the request URL so alert-label
values cannot leak through logs. Unknown response metadata remains ignored for
NetBox forward compatibility.

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
