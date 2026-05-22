# Phase 04: AI Integration

## Goal

Implement the headless LLM report loop first, then validate OpenClaw as a
short-lived sandbox. Interactive diagnosis is a later track.

## 04.1 Headless LLMProvider

Deliverables:

- `LLMProvider` interface
- mock provider
- OpenAI-compatible provider
- prompt templates
- JSON parser and validator
- golden prompt tests

## 04.2 OpenClaw Headless Sandbox PoC

Deliverables:

- Docker `ContainerProvider`
- OpenClaw image bootstrap
- readonly skill directory
- non-root user
- network allowlist
- fixed timeout
- stdout JSON extraction
- cleanup on success, failure, and timeout

## 04.3 Later Interactive Diagnosis Room

This section is not part of the MVP acceptance path.

Later deliverables:

- Next.js diagnosis room
- AuthProvider integration
- RBAC for owner, admin, and leader roles
- WebSocket proxy to sandbox stdin/stdout or PTY
- chat turn persistence
- unsafe instruction filter
- lifecycle-end compression prompt
- final group notification after session expiry

## Acceptance

- headless LLM reports pass golden tests
- OpenClaw headless sandbox can run and clean up deterministically
- interactive diagnosis remains behind the later-track boundary
