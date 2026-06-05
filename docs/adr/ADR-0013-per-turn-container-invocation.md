---
id: ADR-0013
title: "Per-Turn Container Invocation and File-Based Data Contract"
status: "proposed"
date: 2026-05-19
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0013: Per-Turn Container Invocation and File-Based Data Contract

> **Review Period**: Until 2026-05-21 (48-hour minimum)

## Context and Problem Statement

OpenClarion runs AI agents inside sandboxed containers for both batch report
enhancement (M4) and interactive diagnosis (M5). The project must decide:

1. **Invocation model**: should the agent container be long-running (receiving
   messages via stdin/stdout or HTTP) or invoked once per turn?
2. **Data contract**: how do Go and the agent exchange inputs and outputs?

These choices directly affect crash recovery complexity, security isolation,
debugging ergonomics, and latency.

## Decision Drivers

* minimize container lifecycle management complexity
* make crash recovery trivial (replay from last persisted state)
* reuse M4 batch model for M5 interactive turns
* avoid designing a streaming IPC protocol (stdin/stdout) in V1
* keep agent containers stateless (state lives in Temporal workflow)
* support diverse agent runtimes without framework-specific protocols
* enable future optimization (persistent containers) without contract change

## Considered Options

* **Option 1**: Per-turn container invocation with file-based I/O
* **Option 2**: Long-running container with stdin/stdout streaming
* **Option 3**: Long-running container with HTTP endpoint inside

## Decision Outcome

**Chosen option**: "Per-turn container invocation with file-based I/O", because
it eliminates long-running process management, makes crash recovery trivial, and
unifies M4 and M5 under one model.

### Per-Turn Model

Each agent invocation (M4 batch or M5 interactive turn) runs as an independent
container:

```text
Turn N:
  Go prepares input files (readonly bind mounts)
  Go creates container with input mounts + writable capped output mount
  Container starts -> agent reads inputs -> reasons -> writes output
  Container exits
  Go reads /workspace/out/output.json -> validates -> persists
```

For M5, this repeats for each turn in the conversation. Conversation history
grows as Go appends each turn to `conversation.json` before mounting it for
the next container invocation.

### File-Based Data Contract

The contract between the Go control plane and the agent is files, not
stdin/stdout or HTTP:

```text
Input (mounted readonly by Go):
  /workspace/evidence.json         # structured evidence from EvidenceSnapshot
  /workspace/conversation.json     # conversation history (M5 only)
  /workspace/message.json          # latest user message (M5 only)
  /workspace/agent_config/         # agent role, skills, tool endpoints

Output (writable capped mount, read by Go after container exits):
  /workspace/out/output.json       # structured response (schema-validated by Go)
```

Invariants:
* Go prepares input files and mounts them as `:ro` (read-only bind mounts).
* `/workspace/out/` is the only writable mount and is capped by container
  `fsize` ulimit plus Go's output read limit.
* Agent writes ONLY to `/workspace/out/output.json`.
* Go validates `output.json` against the expected JSON Schema before accepting it.
* Go never reads `agent_config/` contents — it mounts the directory opaquely.

Docker Engine implementation note: the provider uses a private per-invocation
host bind mount for `/workspace/out` because Docker archive copy cannot extract
files from container tmpfs mounts reliably after execution.

### M5 Turn-by-Turn Mechanics

```text
Turn 1:
  Go mounts: evidence.json + conversation.json (empty) + message.json (user msg 1)
  Container runs -> agent responds -> /workspace/out/output.json
  Go reads + validates output, persists turn, appends to conversation history

Turn 2:
  Go mounts: evidence.json + conversation.json ([turn1]) + message.json (user msg 2)
  Container runs -> agent sees full history -> responds -> /workspace/out/output.json
  Go reads + validates output, persists turn, appends to conversation history

... repeat until turn limit or session timeout ...
```

### Consequences

* Good, because no long-running container process to manage.
* Good, because crash recovery is trivial (replay from last persisted turn).
* Good, because M4 and M5 share the same container invocation code path.
* Good, because any agent runtime that can read files and write JSON works
  (no framework-specific protocol required).
* Good, because conversation state lives in Temporal workflow (durable), not
  container memory (ephemeral).
* Good, because OpenClaw, Hermes Agent, or a custom thin runner can be evaluated
  as sandbox-image adapters without changing the Go control-plane contract.
* Neutral, because each turn pays ~1-3s container startup cost.
* Bad, because per-turn startup latency is higher than a persistent container.

### Latency Trade-off

Each turn incurs ~1-3s of container startup overhead. This is acceptable for V1
short-conversation diagnosis (bounded turns, human typing cadence). If latency
becomes a concern post-V1:

* Keep the container alive with an HTTP endpoint inside, converting to Option 3.
* This is a `ContainerProvider` implementation detail that does NOT change the
  workflow contract, file paths, or validation logic.
* The file-based contract remains: Go still writes input files and reads
  `output.json`, but the container is pre-started.

### Confirmation

* M4 and M5 container invocation share the same `ContainerProvider.Run()` code
* no stdin/stdout streaming protocol exists in the codebase
* workflow tests verify turn replay from last persisted state
* output is always read from `/workspace/out/output.json`, never from
  container stdout
* integration test: kill container mid-turn -> workflow retries from same input
  -> produces identical output

---

## Pros and Cons of the Options

### Option 1: Per-Turn Container with File-Based I/O (Chosen)

* Good, because stateless containers, trivial crash recovery
* Good, because no IPC protocol design
* Good, because works with any agent runtime
* Bad, because ~1-3s startup latency per turn

### Option 2: Long-Running Container with stdin/stdout

* Good, because low latency after initial startup
* Bad, because requires designing a framing/streaming protocol
* Bad, because container crash requires reconnection and state recovery
* Bad, because container process management is complex (health checks, restarts)
* Bad, because stdin/stdout is not auditable by default

### Option 3: Long-Running Container with HTTP Endpoint

* Good, because low latency, standard protocol
* Good, because health checks are trivial (HTTP GET)
* Bad, because container crash requires restart and state recovery
* Bad, because requires agent runtime to implement an HTTP server
* Neutral, because this is the recommended post-V1 optimization path

---

## More Information

### Related Decisions

* ADR-0002 — agent black-box boundary (this ADR implements the concrete IPC mechanism)
* ADR-0004 — Temporal workflow engine (Updates dispatch turns to per-turn Activities)
* ADR-0005 — ephemeral container security (mount scoping and egress control apply here)
* [Agent Runtime Selection Gate](../design/agent-runtime-selection.md) — M4
  OpenClaw/Hermes/custom thin runner acceptance criteria

### Re-evaluation Trigger

If V1 user testing reveals that per-turn startup latency (1-3s) is unacceptable
for interactive diagnosis UX, Option 3 (persistent container with HTTP endpoint)
should be evaluated. This does not require a new ADR — it is a ContainerProvider
implementation change within the existing contract.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-19 | jindyzhao | Initial proposal |
| 2026-05-28 | jindyzhao | Link M4 runtime selection gate and clarify that OpenClaw/Hermes/custom thin runner adoption is an image-adapter decision, not a control-plane contract change |
| 2026-06-06 | jindyzhao | Clarify that retained M4 runtime-smoke bundles are validated before review handoff, keeping ADR-0013 runtime evidence tied to canonical one-shot file-contract, provider lifecycle, timeout, output-cap, and egress artifacts |
