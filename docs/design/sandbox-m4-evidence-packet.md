# Sandbox M4 Evidence Packet

OpenClarion remains an intelligent alert analysis product. The M4 evidence
packet is a repeatable way to freeze sandbox decision evidence; it does not
create representative samples and it does not accept a runtime by itself.

## Scope

`scripts/sandbox_m4_evidence_packet` assembles one directory containing:

- `baseline-audit.json`: output from `scripts/sandbox_baseline_audit`
- `quality-comparison.json`: output from `scripts/sandbox_quality_compare
  --manifest <file> --fail-on-regression`
- `quality-inputs/quality-manifest.json`: copied quality manifest used to
  generate `quality-comparison.json`
- `quality-inputs/reports/`: copied direct and sandbox SubReport inputs named
  by the quality manifest
- `review-evidence.json`: copied human review evidence file
- `runtime-smoke-artifacts/`: copied runtime-smoke artifacts referenced by
  `review-evidence.json`
- `decision.json`: output from `scripts/sandbox_m4_decision`
- `packet.json`: summary of packet-local artifact paths, SHA-256 artifact
  digests, helper commands, copied quality inputs, copied runtime-smoke
  artifacts, and final decision

The output directory must be empty. This prevents evidence from multiple
candidate runs being accidentally mixed.

## Manual Run

```bash
make sandbox-m4-evidence-packet \
  QUALITY_MANIFEST=artifacts/samples/quality-manifest.json \
  REVIEW_EVIDENCE=artifacts/samples/review-evidence.json \
  RUNTIME_SMOKE_ARTIFACTS_ROOT=artifacts/samples \
  OUT_DIR=artifacts/m4-decision/runtime-candidate-a-2026-05-28
```

After retaining or moving a packet, verify it without rerunning helpers:

```bash
make sandbox-m4-evidence-packet-verify \
  PACKET_DIR=artifacts/m4-decision/runtime-candidate-a-2026-05-28
```

`PACKET_JSON=.../packet.json` is also accepted when a caller wants to verify a
specific summary file.

`runtime-candidate-a` in this path is only an operator-chosen evidence label.
It is not a runtime enum or OpenClarion default.
`RUNTIME_SMOKE_ARTIFACTS_ROOT` is optional; when omitted, the packet helper
resolves runtime-smoke `evidence_ref` paths relative to the review evidence
file's directory.

For the local custom thin runner candidate, operators can produce the runtime
smoke artifact directory while the target's ephemeral localhost registry still
serves the digest-pinned image:

```bash
OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=artifacts/samples/runtime-smokes \
  OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT=artifacts/samples/runtime-smokes/digest-ref.txt \
  make custom-thin-runner-smoke
```

This is only a retained lifecycle/security artifact path. The custom thin
runner remains a candidate proof until a representative quality manifest,
human review evidence, and verified M4 decision packet support a recorded
`proceed` decision.

The quality manifest still owns the real direct/sandbox SubReport file list,
its `sample_basis`, and the per-case alert scenario labels. The decision gate
checks that the generated comparison output covers `single_alert`, `cascade`,
and `alert_storm`. The review evidence still owns runtime smoke sources and
reviewer status, and must repeat the exact quality-comparison `sample_basis`
for human audit context. `make sandbox-m4-runtime-smoke-artifacts` can collect
the five canonical runtime-smoke proof files into one empty directory, and
`make sandbox-m4-review-evidence-template` can produce a fail-closed draft
review-evidence file from the retained quality comparison and those artifacts,
including a retained custom-runner `digest-ref.txt` when
`RUNTIME_CANDIDATE_FILE=...` is set. Operators must still review the draft and
edit candidate, reviewed-case, human-review, and representative-sample fields
before it can support acceptance. Quality manifest report paths are
intentionally
relative to the manifest directory, non-traversing, and distinct per case so
packet artifacts stay portable and cannot compare a report file to itself.
`make sandbox-m4-quality-sample-export` can create that retained sample-root
layout from operator-selected persisted SubReport rows after checking strict
selection JSON, scenario matches, shared EvidenceSnapshot IDs, production
SubReport validity, and an empty output directory. It remains a sample export
step only; the operator still owns sample representativeness and downstream
review.
`make sandbox-m4-quality-manifest-prepare` can create that quality manifest
from retained `direct/<scenario>/<case>.json` and
`sandbox/<scenario>/<case>.json` SubReport pairs after parsing both sides
through the production parser and requiring all three alert-analysis scenarios.
`make manual-evidence-readiness` now preflights that sample-root layout,
paired-case counts, required scenario coverage, `SAMPLE_BASIS`, and the
manifest `OUT` path before operators run the helper. It also preflights the
retained quality-comparison manifest and `OUT` path for
`make sandbox-m4-quality-compare`, which runs manifest comparison with
fail-on-regression and writes the retained comparison JSON without overwriting
an existing file. This is still preparation only; the sample still needs
operator selection, quality comparison, human review, and packet verification
before it can support a decision.
The packet helper validates that the copied manifest's `sample_basis`, case
IDs, scenarios, and `required_evidence_refs` match the generated quality
comparison output, then validates every copied direct/sandbox SubReport with
the production SubReport parser and requires both reports to contain all
declared refs before freezing them under `quality-inputs/reports/`.
Runtime smoke sources are validated by
`scripts/sandbox_m4_decision` against the canonical `make` targets documented in
[sandbox-m4-decision.md](sandbox-m4-decision.md). The packet helper resolves
each runtime-smoke `evidence_ref` under `RUNTIME_SMOKE_ARTIFACTS_ROOT` or the
review evidence directory, verifies that the source file's SHA-256 matches
`evidence_sha256`, and copies the artifact into
`runtime-smoke-artifacts/<evidence_ref>` inside the empty packet directory.
Review evidence must also include case-level `reviewed_cases` entries that
correspond to the generated quality cases; the decision output records
`reviewed_case_count` and the packet validator cross-checks both the count and
the reviewed case IDs against the review evidence and quality comparison. The
packet helper only freezes those inputs with fresh baseline, comparison,
quality-input copies, runtime-smoke artifact copies, and decision outputs.
Before writing artifacts, the packet helper also validates their minimum shape:
baseline output must include uniquely named required pass checks, quality
output must be manifest-mode with the current SubReport schema identifier,
`sample_basis`, required scenario coverage derived from its cases, unique case
IDs, per-case `review_required`, non-empty per-case `required_evidence_refs`
with a canonical `snapshot:<positive-id>` EvidenceSnapshot ref, and summary
counts that match `cases[].recommendation`. Quality case IDs must be
single-line, unpadded, and no more than 128 bytes; retained required evidence
refs must be single-line and no more than 120 runes.
The packet helper also checks that the batch recommendation follows the
case-derived summary before writing the quality artifact. Review evidence must
include a non-future dated representative sample basis, an operator-supplied
`selected_candidate`, at least one candidate evaluation with the selected
candidate marked `pass`, an immutable `name@sha256:<64-hex-digest>` runtime
candidate, and a matching immutable runtime ref on the selected candidate
evaluation. Any pass-status candidate evaluation must cite all required
runtime smoke names through `runtime_smoke_refs`, so the chosen generic
candidate is tied to the same canonical runtime, Provider lifecycle, timeout,
output-cap, and egress proofs that the decision gate evaluates. It also
requires runtime smoke names to be exactly the required smoke set, canonical
runtime smoke sources, distinct bounded single-line normalized relative
`evidence_ref` values with no absolute paths, traversal, URI syntax,
backslashes, or spaces, 64-character lowercase `evidence_sha256` values for
retained smoke artifacts/logs, bounded `pass` / `fail` statuses, case-level
reviewed-case notes, human review reviewer/status/notes metadata, and the same
`sample_basis` as the generated quality comparison. Candidate IDs
must be unpadded, contain no whitespace, and be no more than 128 bytes; reviewed
case IDs must be single-line, unpadded, and no more than 128 bytes.
Human-authored
review evidence text in `sample_basis`, human review metadata, candidate
evaluation source/notes, and reviewed-case notes must be single-line, free of
leading/trailing whitespace, and no more than 2048 bytes per field before the
packet helper writes any artifacts.
Decision output must use
`proceed`, `iterate`, or `defer` with review required and decision-consistent
reasons. A `proceed` decision must carry only the canonical success reason;
`iterate` and `defer` decisions must not reuse that success reason, and blank,
whitespace-padded, or duplicate reasons are rejected. Its evidence summary must
match the frozen baseline, quality, review, and `--min-cases` inputs in the
same packet.
Helper output and copied review evidence also reject duplicate JSON object keys
and unknown fields before parsing so retained artifacts cannot depend on
last-key-wins behavior or unvalidated proof claims.
`packet.json` records SHA-256 digests for `baseline-audit.json`,
`quality-comparison.json`, `review-evidence.json`, and `decision.json`, so a
retained packet binds its summary to the exact frozen evidence files rather
than only to local paths. It records `out_dir` as `.` and stores generated
artifact paths and helper `output_path` fields as slash-separated paths
relative to the packet root. It also records each copied runtime-smoke
artifact's name, original `evidence_ref`, packet-local path, and SHA-256
digest, plus the copied quality manifest and every copied direct/sandbox
SubReport input path and digest. A retained packet therefore cannot depend on
missing external quality inputs, missing external smoke logs, or a
machine-specific output directory.
During assembly, source quality manifests, review evidence files, direct/sandbox
SubReports, and runtime-smoke artifacts must be regular files; symlinks and
other non-regular files are rejected before hashing or copying.
The same helper also supports `--verify-packet <dir|packet.json>`, which
revalidates an existing retained packet offline. Verification rejects stale
packet metadata, non-local command paths, mismatched helper command arguments,
core artifact digest drift, quality manifest/output drift, copied SubReport
schema or evidence-ref failures, runtime-smoke artifact digest drift, and
decision evidence that no longer matches the packet-local artifacts. It also
walks the packet directory and rejects unexpected files, unexpected
directories, symlinks, non-regular files, or a `PACKET_JSON` path that does not
match the packet's own `artifacts.packet` reference, keeping retained packets
as closed evidence directories instead of loose folders with sidecar claims.

## Validation

```bash
make sandbox-m4-evidence-packet-test
```

The test target is part of `make ci`. The manual packet target is not run in CI
because it requires representative sample artifacts and human review evidence.
