# Agent Runtime Selection Gate

## Status

M4 decision gate and selected M5 runner implementation. Last reviewed:
2026-07-13.

## Purpose

M4 and M5 need an agent runtime inside the sandbox image, but the Go control
plane must not grow a home-built agent framework by accident. This gate defines
what must be proven before OpenClarion accepts any sandbox agent runtime. Eino
is selected for the M5 runner implementation; OpenClaw, Hermes Agent, and the
custom thin runner remain comparison points rather than platform bindings.

OpenClarion's product direction remains intelligent alert analysis. Runtime
selection is an implementation boundary for alert evidence analysis and
diagnosis turns; it is not a repositioning into a generic agent platform.

The current decision is **Eino-backed adapter-first**:

- keep `ContainerProvider` and the
  [ADR-0013 file contract](../adr/ADR-0013-per-turn-container-invocation.md)
  as the stable boundary
- pin CloudWeGo Eino `v0.9.12` inside the isolated diagnosis runner for the
  Agent lifecycle, message mapping, bounded iteration, and cancellation
- keep Eino out of the root Go module and control-plane service binary
- do not implement a custom planner, memory layer, multi-agent router, or tool
  approval system
- keep Temporal/PostgreSQL as the durable conversation owner and disable
  framework checkpoint/session persistence

## Non-Negotiable Contract

Every candidate runtime must satisfy these invariants before it can become the
M4/M5 baseline:

| Area | Requirement |
|------|-------------|
| Invocation | one short-lived container invocation per M4 group or M5 turn |
| Input | reads `/workspace/evidence.json`, optional `/workspace/conversation.json`, optional `/workspace/message.json`, and opaque `/workspace/agent_config/` |
| Output | writes only `/workspace/out/output.json`; stdout/stderr are diagnostic only |
| Validation | output is valid JSON and passes the caller-owned JSON Schema before persistence |
| State | durable state stays in PostgreSQL/Temporal; candidate-local memory is disabled or confined to invocation-scoped tmpfs for V1 |
| Filesystem | non-root user, readonly rootfs, readonly input mounts, writable capped output mount only at `/workspace/out` |
| Network | default network-none; allowlist mode must be enforced by Docker network plus egress proxy or equivalent |
| Credentials | short-lived credentials only, TTL no longer than the container timeout |
| Lifecycle | honors SIGTERM, exits within the timeout, and is always cleaned up on success, failure, and cancellation |
| Tool policy | tool execution is deny-by-default; write/edit/browser/host gateway tools are unavailable unless explicitly accepted by a future ADR/update |

## Candidate Examples

These notes began with the 2026-05-28 Context7 review. Eino was reviewed again
through Context7 and the pinned `v0.9.12` source on 2026-07-13. New candidates
can be evaluated by adding evidence without changing the control-plane
contract.

| Candidate | Fit | Integration Questions |
|-----------|-----|-----------------------|
| Eino `v0.9.12` | **Selected for M5 implementation.** Native Go ADK with bounded ChatModelAgent iterations, context cancellation, middleware boundaries, and Apache-2.0 licensing. It embeds in a short-lived process without an always-on gateway. | OpenClarion supplies prior messages on every invocation, registers no V1 tools, and retains provider, validation, persistence, and approval policy outside Eino. |
| OpenClaw | Deferred. Its gateway, channel, session, and tool surfaces are larger than the accepted one-turn sandbox boundary. | Reconsider only if a future requirement cannot remain in OpenClarion's control plane. |
| Hermes Agent | Deferred. Its bundled memory, skills, and broad toolsets create more deny-by-default work than the embedded Go path. | Reconsider only if the selected runner fails representative diagnosis quality or lifecycle evidence. |
| Custom Agent loop | Rejected for M5. It would duplicate lifecycle and future tool-call behavior maintained by Eino. | Keep only protocol adapters, validation, security projection, and file I/O around the framework. |

## Selection Procedure

Before M4 acceptance, run each candidate through the same smoke:

1. Build a digest-pinned sandbox image for the candidate.
2. Run it through `make agent-runtime-smoke` with network-none and readonly
   input mounts; then run the same image through `ContainerProvider.Run` with
   `make container-provider-smoke`.
3. Verify it writes duplicate-key-free valid JSON to `/workspace/out/output.json`.
4. Verify it cannot write outside `/workspace/out`.
5. Verify timeout cancellation triggers stop/remove cleanup.
6. Verify SIGTERM is handled or force-kill cleanup leaves no container.
7. Verify any required LLM/tool credentials are short-lived and not logged.
8. Compare the resulting report against the M2 direct LLM baseline.

Eino `v0.9.12` is the selected framework for the M5 runner implementation.
The pinned-source review covered `adk/chatmodel.go`, `adk/runner.go`,
`adk/react.go`, and
`schema/stream.go`; the isolated module is included in tests, vulnerability
scanning, license checks, and Dependabot. This selection does not accept Eino
as the M4 report-enhancement quality baseline. M4 still requires its retained
decision packet and representative direct-versus-sandbox evidence.

## Bundled Diagnosis Runner

`scripts/diagnosis_assistant_runner` is an isolated Go module that maps the
mounted evidence, prior transcript, and latest user message into one Eino
ChatModelAgent invocation. It uses the existing OpenAI-compatible provider,
strict diagnosis schema, idempotency key, and bounded validation retry, then
writes only schema-valid `/workspace/out/output.json`.

V1 registers no Eino tools, framework persistence, multi-agent router, or
custom Agent loop. Inputs are direct regular files with explicit size limits;
output publication refuses existing files and uses an exclusive temporary file
inside the output root. The tracked safe prompt is
`config/agents/diagnosis-assistant/instructions.md`.

Run the module checks with `make diagnosis-agent-runtime-check`. Build the
non-root `scratch` image and print its local immutable reference with:

```bash
make diagnosis-assistant-runner-build
```

The image includes a CA bundle and generated third-party license material. The
build defaults to `linux/amd64`; set
`OPENCLARION_DIAGNOSIS_RUNNER_TARGET_ARCH=arm64` for an explicit ARM64 target.
Only those two architectures are accepted, and the helper verifies that the
image metadata matches the cross-compiled binary target. The build helper uses
a temporary loopback registry to resolve a real repository digest; a repo-local
digest-ref output must be ignored and cannot overlap a tracked path.
ContainerProvider, Temporal, WebSocket, and browser integration
remain separate follow-up concerns and are not claimed by this runner batch.

## Manual Smoke Harness

`make agent-runtime-smoke` is the local candidate-image harness. It is not part
of `make ci` because it requires a real candidate image and, for framework
candidates, provider credentials or mocked provider endpoints.

Required:

```text
OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/agent@sha256:<64-hex-digest>
```

Optional:

| Variable | Default | Purpose |
|----------|---------|---------|
| `OPENCLARION_AGENT_RUNTIME_TIMEOUT_SECONDS` | `60` | hard wait cap for the candidate container |
| `OPENCLARION_AGENT_RUNTIME_PULL` | `missing` | Docker pull policy: `always`, `missing`, or `never` |
| `OPENCLARION_AGENT_RUNTIME_EVIDENCE_PATH` | generated fixture | evidence input mounted at `/workspace/evidence.json` |
| `OPENCLARION_AGENT_RUNTIME_CONVERSATION_PATH` | generated fixture | conversation input mounted at `/workspace/conversation.json` |
| `OPENCLARION_AGENT_RUNTIME_MESSAGE_PATH` | generated fixture | latest message input mounted at `/workspace/message.json` |
| `OPENCLARION_AGENT_RUNTIME_AGENT_CONFIG_DIR` | generated fixture | agent config mounted at `/workspace/agent_config` |
| `OPENCLARION_AGENT_RUNTIME_OUTPUT_PATH` | temp file | copied output JSON path on the host |
| `OPENCLARION_AGENT_RUNTIME_PROOF_PATH` | temp file | retained smoke proof JSON path on the host |
| `OPENCLARION_AGENT_RUNTIME_SHOW_LOGS` | unset | print tail logs on failure; use only in a controlled shell |
| `OPENCLARION_AGENT_RUNTIME_SHELL_COMMAND` | unset | optional `sh -c` command override for validating generic smoke images; candidate images normally use their own entrypoint |

The harness creates the container with the same security posture required by
the Docker security spec: digest-pinned image, `--network none`, non-root user,
readonly rootfs, `no-new-privileges`, `--cap-drop ALL`, memory/CPU/PID limits,
readonly input bind mounts, `/workspace/out` as the only writable bind mount,
and an `fsize` ulimit matching the 10 MiB output cap. It copies
`/workspace/out/output.json` from the stopped container and validates that the
file is a non-empty JSON object under the cap with no duplicate object keys or
trailing JSON values. It also writes a runtime-agnostic proof JSON when the
output is valid. The proof records `tool`, `status`, canonical source
`make agent-runtime-smoke`, the digest-pinned `runtime_candidate`, container
output path `/workspace/out/output.json`, output byte count, output SHA-256,
configured output cap, and the checks that passed. It deliberately omits host
input/output paths so the artifact can be retained under
`runtime_smokes[].evidence_ref` without leaking operator-local filesystem
details.

`make container-provider-smoke` is the companion Provider smoke. It invokes the
Docker-backed `ContainerProvider.Run` path through the Go control plane against
a real local Docker daemon, validates duplicate-key-free JSON-object output,
and fails if the invocation-labelled container remains after cleanup. Without
an explicit candidate image, it resolves `busybox:1.36.1` to a repo digest and runs a
minimal command that reads the mounted ADR-0013 inputs and writes
`/workspace/out/output.json`; accepted runtime candidates must still run with
their own digest-pinned image.

`make container-provider-timeout-smoke` runs the same Provider path with a short
timeout and a long-running command. It expects the timeout error and then checks
the Docker label set to prove the container was removed.

`make container-provider-output-cap-smoke` runs the Provider with a small output
cap and a command that writes more than the cap. The accepted failure is either
the container process failing under `fsize` or the Provider rejecting copied
output as too large; both paths must still remove the container.

`make custom-thin-runner-smoke` is the first concrete candidate-runtime proof.
It builds the local scratch-based custom runner, pushes it to an ephemeral
localhost registry to obtain a real `repo@sha256` reference, and then runs the
same image through both `make agent-runtime-smoke` and
`make container-provider-smoke`. The runner only reads ADR-0013 inputs and
writes a JSON contract proof. It rejects symlink/non-regular ADR-0013 JSON
input paths and duplicate-key or trailing-value input JSON before hashing
mounted files. The same image also packages the metric/topology helper binaries
and proves the topology helper with
`docker run --entrypoint` under the same non-root, readonly, network-none
posture. It is a lifecycle and file-contract result, not an accepted
report-quality baseline.

For retained review evidence, the same target can keep the ephemeral registry
alive long enough to run the canonical artifact bundle:

```bash
OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=artifacts/m4-runtime-smokes/custom-thin-runner \
  OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT=artifacts/m4-runtime-smokes/custom-thin-runner/digest-ref.txt \
  make custom-thin-runner-smoke
```

The artifacts directory must be absent or empty because the underlying
`make sandbox-m4-runtime-smoke-artifacts` target owns the same mixing guard as
manual candidate runs. The digest-ref file is optional and is written only when
the path does not already exist. When either retained output path is inside the
repository, the smoke helper requires it to be ignored by git and not overlap
tracked files. `make sandbox-m4-review-evidence-template` can consume that file
through `RUNTIME_CANDIDATE_FILE=...`, avoiding manual digest copy/paste while
preserving the same immutable image-reference validation. These options only
retain lifecycle/security proofs for later review-evidence generation; they do
not make the sample representative, accept the custom
runner as the runtime baseline, or replace direct-vs-sandbox report-quality
comparison.

`make agent-tool-scripts-test` proves the first tool-helper contracts that
candidate images may package later. The helpers are intentionally read-only:
one bounded Prometheus instant-query helper and one bounded static topology
lookup helper. They do not change the control-plane dependency boundary.

`make sandbox-baseline-audit` proves the code-level M4/M5 sandbox baseline
without needing a Docker daemon. It builds provider-neutral requests and Docker
runtime specs for batch and M5 turn shapes, then checks fixed file paths,
read-only inputs, writable output, network-none defaults, allowlist subset
enforcement, non-root/readonly/no-new-privileges/capability-drop posture,
resource limits, and raw JSON output validation.

`make sandbox-quality-compare-test` proves the offline comparison helper used
after a candidate produces a sandbox SubReport. It validates the direct M2
SubReport and sandbox SubReport with the production report draft parser before
emitting conservative deltas. Manifest mode can compare a batch of
representative direct/sandbox sample pairs, require `sample_basis` and per-case
alert scenario labels plus `required_evidence_refs`, and emit aggregate counts
plus `scenario_coverage`. Each manifest case must include a canonical
`snapshot:<positive-id>` EvidenceSnapshot ref and prove both direct and sandbox
SubReports still contain the declared evidence refs, keeping the runtime
decision measurable while leaving the real report-quality judgment pending
until representative alert evidence is run through candidate images.
`make sandbox-m4-subreport-generate` is the manual bridge from a persisted
`EvidenceSnapshot` to a persisted sandbox candidate `SubReport`. It wraps the
snapshot payload with snapshot ID, canonical `snapshot:<id>` ref, digest,
scenario, and group metadata, runs the configured digest-pinned candidate image
through the existing Docker `ContainerProvider`, validates `output.json`
through the production SubReport parser, requires the canonical snapshot ref in
`evidence_refs`, and writes an idempotent sandbox row keyed by snapshot, group,
and candidate ID. It does not generate the direct baseline, select
representative samples, compare quality, or accept the runtime.
`make sandbox-m4-quality-sample-export` is the manual bridge from real
persisted direct/sandbox SubReport rows to that retained sample layout. It
uses an operator-authored selection file, reads PostgreSQL through
`DATABASE_URL`, rejects duplicate case/report IDs, scenario mismatches, mixed
EvidenceSnapshot IDs, invalid persisted SubReport JSON, and non-empty output
roots, and writes only `direct/<scenario>/<case>.json` plus
`sandbox/<scenario>/<case>.json` samples. It does not decide sample
representativeness or report quality.
`make sandbox-m4-quality-manifest-prepare` is the manual preparation path for
that manifest: it scans retained `direct/<scenario>/<case>.json` and
`sandbox/<scenario>/<case>.json` SubReport pairs, requires all three
alert-analysis scenarios, parses both sides through the production parser, and
derives portable required evidence refs from refs shared by both reports. It
does not make a sample representative or accept a runtime; it only reduces
pairing and evidence-ref mistakes before the quality comparison runs.

`make sandbox-m4-decision-test` proves the offline decision helper that later
combines the baseline audit, manifest-mode quality comparison, runtime smoke
evidence, and human review evidence. It requires quality evidence to cover
`single_alert`, `cascade`, and `alert_storm`, and requires review evidence to
name the same `sample_basis` as the quality comparison before proceeding. It
also requires quality cases to retain non-empty `required_evidence_refs` with a
canonical `snapshot:<positive-id>` EvidenceSnapshot ref,
requires review evidence to cover the same case IDs, and recomputes aggregate
summary counts from `cases[].recommendation` so a retained quality artifact
cannot hide a regressed alert-analysis case by editing only top-level summary
fields. Runtime candidates remain evidence-supplied IDs; a pass-status
candidate evaluation must bind its immutable runtime image reference and cite
every required runtime smoke name through `runtime_smoke_refs`, so accepting a
candidate does not require hard-coding OpenClaw, Hermes Agent, or any other
runtime family into the Go control plane. Each runtime smoke claim must also
carry a retained artifact/log `evidence_ref` and lowercase SHA-256 digest; the
ref must be a normalized relative artifact path, not an absolute path, traversal
path, URI, or local-machine-specific path. The evidence packet helper verifies
those artifact files against the declared digests and copies them into the
packet before writing `packet.json`; the decision therefore records portable
evidence pointers rather than only pass/fail prose. The manual
`make sandbox-m4-decision` target is the auditable proceed/iterate/defer
recording path once real candidate evidence exists.

`make sandbox-m4-evidence-packet-test` proves the packet assembler and retained
packet verifier. The assembler freezes baseline audit, quality comparison,
copied quality manifest/direct/sandbox SubReport inputs, copied review
evidence, copied runtime-smoke artifacts, decision output, and packet summary
into one empty artifact directory. It also proves weak generated helper
artifacts and weak review evidence are rejected before they can be written,
including duplicate quality case IDs, quality summaries that do not match case
recommendations, review evidence for a different `sample_basis`, missing or
duplicated reviewed cases, selected pass-candidate runtime-smoke refs that do
not cover every required smoke name, runtime smoke claims without bounded
normalized evidence refs or SHA-256 digests, copied report inputs that fail the
production SubReport parser or miss declared refs, and decision evidence that
does not match the frozen packet inputs. The packet summary records SHA-256
digests for the frozen baseline, quality, review, decision, quality-input, and
runtime-smoke artifacts. `make sandbox-m4-evidence-packet-verify` revalidates a
retained packet offline without rerunning helpers and rejects unexpected sidecar
files or directories. The manual packet target is
the preferred handoff artifact for M4 runtime baseline review.
`make sandbox-m4-runtime-smoke-artifacts` also invokes
`scripts/sandbox_m4_runtime_smoke_artifacts` after the five Docker-backed
smokes complete, so the retained bundle fails before handoff if any artifact is
missing, extra, symlinked, duplicate-key JSON, from the wrong canonical
`make` source, not pass-status, not digest-pinned, or inconsistent with the
success versus expected-error mode required for that smoke.

## Control-Plane Dependency and Hardcoding Rule

The root Go module and first-party frontend packages must not add runtime
families listed in [agent-runtime-forbidden.tsv](ci/agent-runtime-forbidden.tsv)
or equivalent agent framework dependencies before this gate records an accepted
baseline. Candidate-specific runtime dependencies belong inside the sandbox
image build context and must be referenced by digest-pinned images.

First-party non-test control-plane source must also not hard-code those
runtime-family names. The current scan covers Go, shell, and frontend
JavaScript/TypeScript source under `cmd/`, `internal/`, `scripts/`, and
`web/src/`, while keeping docs and test fixtures available for evaluation
evidence. Candidate names belong in docs, operator evidence, retained review
packets, and sandbox image build contexts until the runtime selection gate
accepts a baseline and the policy table is updated intentionally.

`make forbidden-agent-runtime` enforces these rules for first-party dependency
manifests and non-test first-party source by reading
`docs/design/ci/agent-runtime-forbidden.tsv`. This keeps framework-specific
names in an auditable governance policy instead of embedding them in the gate
script or production code. The policy table itself is validated by the gate:
rows must have `manifest` or `code` scope, non-empty unpadded patterns, at
least one entry for each scope, and no duplicate scope/pattern pairs.
Manifest scanning is structural: `go.mod` is parsed as module metadata and
`package.json` checks only dependency sections. Go source scanning uses the Go
parser for import paths, string literals, and identifiers, while shell and
frontend JavaScript/TypeScript files remain text-scanned because they are
runtime-branch surfaces until the M4 decision gate accepts a baseline.
