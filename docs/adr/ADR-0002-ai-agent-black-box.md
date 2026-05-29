---
id: ADR-0002
title: "AI Agent Black-Box Runtime Boundary"
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0002: AI Agent Black-Box Runtime Boundary

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion needs AI-assisted diagnosis and reporting, but agent runtimes evolve
quickly and may carry powerful tool execution models. The project must decide
whether to modify agent internals or treat agents as replaceable black-box
runtimes, and must define a clear boundary between the Go control plane and the
agent framework running inside containers.

## Decision Drivers

* preserve upstream upgrade paths
* keep Go control flow deterministic
* allow multiple LLM or agent backends
* constrain tool execution and output parsing
* support testable fallback providers
* define unambiguous responsibility boundary between control plane and agent

## Decision Outcome

**Chosen option**: use AI agents as black-box runtimes behind an `LLMProvider` or
sandboxed `ContainerProvider`. The Go control plane owns lifecycle, validation,
persistence, security, and data flow. The agent framework (inside the container)
owns role, skills, tools, reasoning strategy, and sub-Agent composition.
OpenClarion does not modify agent internals.

### Control Plane vs Agent Framework Boundary

| Layer | Owns | Does NOT Own |
|-------|------|--------------|
| **Go + Temporal** (Control Plane) | when to invoke agent; what data to pass; lifecycle (timeout, cleanup, crash recovery); output validation; persistence; human message relay; security perimeter | which skills the agent loads; what data sources agent connects to internally; how agent reasons; sub-Agent delegation |
| **Agent Framework** (Inside Container) | role definition (system prompt, persona); skills/tools loading; data source connections; reasoning strategy (ReAct, plan-and-execute, etc.); sub-Agent composition; internal state within a single invocation | when it gets invoked; where input comes from; where output goes; its own lifecycle; cross-turn persistence |

### Agent Configuration Independence

Agent configurations live in `agents/` (version-controlled, outside Go code).
Go mounts `agent_config/` into the container as a read-only bind mount but
**never reads or interprets its contents**. Changing an agent's skills, tool
endpoints, or prompts does not require modifying Go code, Temporal workflows,
or redeploying the control plane.

### Consequences

* Good, because agent upgrades do not require a fork.
* Good, because direct LLM providers can validate the reporting loop before any
  agent sandbox runtime is stable.
* Good, because agent developers iterate independently of control plane releases.
* Neutral, because output contracts (JSON Schema) become first-class artifacts.
* Bad, because black-box behavior can vary; golden prompt tests and structured
  output validation are mandatory mitigations.

### Confirmation

* core packages do not import agent runtime internals
* report generation goes through `LLMProvider` or `ContainerProvider`
* all AI output is parsed and validated before persistence
* Go code does not read or parse files under `agents/` or `agent_config/`
* agent configuration changes do not trigger Go recompilation or redeployment

## More Information

### Related Decisions

* ADR-0005 — security constraints for sandboxed containers
* ADR-0013 — per-turn container invocation model and file-based data contract

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
| 2026-05-19 | jindyzhao | Add Control Plane vs Agent Framework Boundary; clarify Go does not own prompts/skills; add agent config independence |
