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

The provider phase now has two responsibilities:

- keep concrete provider adapters small and testable with fakes
- expose alert source connection metadata through backend-owned profiles rather
  than hard-coded frontend constants or customer endpoints in tests

Prometheus is the first implemented alert source adapter. Alertmanager must
land as its own adapter with contract tests and fake coverage before product
documentation or UI can claim Alertmanager support. Both adapters must translate
upstream payloads into the same `ActiveAlert` / `AlertEvent` path.

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
