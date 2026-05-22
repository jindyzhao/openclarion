# Phase 02: Providers

## Goal

Implement default and fake providers for local development and workflow tests.

## Default Providers

| Provider | Default target |
|----------|----------------|
| MetricsProvider | Prometheus alerts, Alertmanager contract placeholder |
| CMDBProvider | static YAML and generic HTTP adapter |
| IMProvider | Email, Webhook, Slack |
| AuthProvider | local development auth, OIDC later |
| ApprovalProvider | no-op and manual approval |
| ContainerProvider | Docker local |
| LLMProvider | mock and OpenAI-compatible endpoint |

## Provider Rules

- Provider implementations must not leak credentials through logs.
- Provider errors must be wrapped with context and mapped to safe user-facing
  messages at API boundaries.
- Provider tests must not require production systems.

## Acceptance

- fake providers cover workflow tests
- default providers support local MVP replay
- provider interfaces are stable enough for P0/P1
