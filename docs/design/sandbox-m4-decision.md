# Sandbox M4 Decision Gate

OpenClarion remains an intelligent alert analysis product. The M4 decision
gate exists to make sandbox-runtime acceptance auditable; it does not select a
runtime by itself and it does not turn sandbox analysis into an automatic
business decision.

## Scope

`scripts/sandbox_m4_decision` combines three operator-supplied JSON evidence
files:

- `--baseline-audit`: output from `make sandbox-baseline-audit`
  or retained `make sandbox-m4-baseline-audit OUT=...`
- `--quality-comparison`: output from `scripts/sandbox_quality_compare
  --manifest <file>` over representative direct/sandbox SubReport pairs
- `--review-evidence`: human review and runtime smoke evidence for the
  candidate runtime

The tool emits one JSON object with:

- `decision`: `proceed`, `iterate`, or `defer`
- `review_required: true`
- summarized evidence fields
- concrete reasons when the result is not `proceed`

`--fail-unless proceed` can be used by an operator to make the command fail
closed unless all required evidence supports proceeding.

The quality-comparison evidence must be generated from a manifest that records
`sample_basis` and labels each case with an alert-analysis scenario. The
decision gate derives coverage from the cases and requires all three M2 prompt
scenarios (`single_alert`, `cascade`, and `alert_storm`) before it can return
`proceed`. The quality output must declare the current SubReport schema
identifier and every case must remain review-marked with non-empty
`required_evidence_refs` that include a canonical `snapshot:<positive-id>`
EvidenceSnapshot ref. Quality case IDs must be single-line, unpadded, and no
more than 128 bytes; retained required evidence refs must be single-line and no
more than 120 runes. These checks keep a stale or hand-edited quality artifact
from bypassing the production parser contract or detaching a comparison case
from its frozen evidence context. The gate also recomputes aggregate counts from
`cases[].recommendation` and rejects quality artifacts whose `summary` does not
match the case-derived counts, so editing only top-level fields cannot hide a
regressed alert-analysis case. The review evidence `sample_basis` must exactly
match the quality-comparison
`sample_basis`; reviewer commentary belongs in `human_review.notes` so stale
review evidence cannot be attached to a different quality sample. Review
evidence must also contain one `reviewed_cases` entry for each quality case and
no extra case IDs. A missing or stale case keeps the decision at `defer`; any
case-level review failure makes the decision `iterate`.
All evidence inputs reject duplicate JSON object keys and unknown fields before
unmarshalling, so retained artifacts cannot hide stale fields behind later
duplicate fields or attach unvalidated proof claims. Baseline audit check names
must also be unique, so a hand-edited audit cannot rely on map overwrite
behavior to hide a failed or missing required check.
All three evidence inputs must be regular files; symlinks and other non-regular
files are rejected before parsing so a manual decision cannot be based on
indirect evidence paths outside the reviewed artifact set.

## Review Evidence Shape

The review evidence file is intentionally separate from generated tool output.
It records the human and runtime-smoke claims that cannot be proven by a pure
offline comparator. Each runtime-smoke `source` must be the canonical `make`
target shown below, not free-form notes, so the evidence remains reproducible.
Each `runtime_smokes[]` item must also retain a bounded single-line
`evidence_ref` plus a 64-character lowercase `evidence_sha256` for the captured
smoke output or log; reused evidence refs are rejected. `evidence_ref` must be a
portable normalized relative artifact path, so absolute paths, path traversal,
URI-style refs, backslashes, and spaces are rejected. `runtime_smokes[].name`
must be one of the required runtime-smoke names; extra manual or ad hoc smoke
names are rejected instead of being retained in the evidence file.
`selected_candidate` and `candidate_evaluations[].candidate` are stable
operator-supplied candidate IDs, not code-defined runtime names. They must be
unpadded, contain no whitespace, and be no more than 128 bytes. The decision
gate only requires that the selected candidate appears in
`candidate_evaluations` with status `pass`; other candidates and their
fit/failure notes remain evidence, not product logic. A candidate evaluation
with status `pass` must include its own immutable `runtime_candidate`, and the
selected candidate's evaluation `runtime_candidate` must exactly match the
top-level review evidence `runtime_candidate`. A pass-status candidate
evaluation must also include `runtime_smoke_refs` covering every required
runtime-smoke name, binding the selected candidate to the canonical runtime,
provider lifecycle, timeout cleanup, output-cap, and egress smoke artifacts
without hard-coding candidate runtime names into the control plane.
`evidence_date` must be a non-future `YYYY-MM-DD` date, `runtime_candidate`
values must be immutable `name@sha256:<64-hex-digest>` image references,
runtime/human statuses must use the bounded `pass` / `fail` vocabulary, and
candidate-evaluation statuses must use `pass`, `fail`, or `not_fit`. Loopback
or `localhost` registry references are accepted only as local smoke evidence:
they force `defer` and cannot support a `proceed` runtime baseline.
`human_review.notes` is required and must explain the reviewer judgement; a
bare pass/fail status is not enough to support retained evidence. Each
`reviewed_cases[]` item must match a quality comparison case ID, carry a
bounded `pass` / `fail` status, include reviewer notes for that specific case,
and use a single-line, unpadded, no-more-than-128-byte case ID. Human-authored
text in `sample_basis`, `human_review.reviewer`,
`human_review.notes`, `candidate_evaluations[].source`,
`candidate_evaluations[].notes`, and `reviewed_cases[].notes` must be
single-line, free of leading/trailing whitespace, and no more than 2048 bytes
per field.

Operators can generate a fail-closed draft review-evidence file from retained
quality comparison output and runtime-smoke artifacts:

```bash
make sandbox-m4-runtime-smoke-artifacts \
  OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=artifacts/m4/runtime \
  OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef

make sandbox-m4-review-evidence-template \
  QUALITY_COMPARISON=artifacts/samples/quality-comparison.json \
  RUNTIME_SMOKE_ARTIFACTS_ROOT=artifacts/m4/runtime \
  SELECTED_CANDIDATE=runtime-candidate-a \
  RUNTIME_CANDIDATE=registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
  REVIEWER=openclarion-maintainer \
  OUT=artifacts/samples/review-evidence.json
```

When the retained runtime-smoke artifacts came from `make custom-thin-runner-smoke`
with `OPENCLARION_CUSTOM_THIN_RUNNER_DIGEST_REF_OUT=.../digest-ref.txt`, use
`RUNTIME_CANDIDATE_FILE=.../digest-ref.txt` instead of manually copying the
digest into `RUNTIME_CANDIDATE`. The custom-runner digest comes from an
ephemeral loopback registry, so it is useful for retained local smoke review but
cannot by itself support a `proceed` decision.

`make sandbox-m4-runtime-smoke-artifacts` is a manual Docker-backed convenience
target. It runs the existing candidate runtime file-contract smoke, candidate
Provider lifecycle smoke, Provider timeout cleanup smoke, Provider output-cap
smoke, and egress allow/deny smoke, retaining the five canonical artifact
filenames expected by the review-evidence template. The Provider timeout,
output-cap, and egress proofs remain boundary proofs and use their existing
smoke harness images unless the operator explicitly overrides those harnesses.

The generator copies the quality `sample_basis` and case IDs, fills the
canonical runtime-smoke names/sources, reads each retained smoke artifact
status, and records SHA-256 digests for those files. It does not mark the
candidate accepted: generated candidate, reviewed-case, and human-review
statuses are `fail` by default, and `representative_sample` remains false
unless the operator explicitly sets `REPRESENTATIVE_SAMPLE=1` after confirming
the sample basis.

The example below uses `runtime-candidate-a` only as a placeholder evidence ID.
It is not a built-in runtime family, product default, or control-plane branch.

```json
{
  "tool": "sandbox_m4_review_evidence",
  "evidence_date": "2026-05-28",
  "selected_candidate": "runtime-candidate-a",
  "runtime_candidate": "registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "representative_sample": true,
  "sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
  "candidate_evaluations": [
    {
      "candidate": "runtime-a",
      "status": "not_fit",
      "source": "candidate smoke review",
      "notes": "candidate still needs a bounded one-shot JSON-file proof"
    },
    {
      "candidate": "runtime-b",
      "status": "fail",
      "source": "candidate smoke review",
      "notes": "candidate did not satisfy the current readonly file-contract smoke"
    },
    {
      "candidate": "runtime-candidate-a",
      "status": "pass",
      "runtime_candidate": "registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "runtime_smoke_refs": [
        "candidate_runtime_file_contract",
        "container_provider_lifecycle",
        "container_provider_timeout_cleanup",
        "container_provider_output_cap",
        "egress_allowdeny"
      ],
      "source": "candidate runtime smoke review",
      "notes": "candidate runtime passed contract and lifecycle smoke as retained review evidence"
    }
  ],
  "runtime_smokes": [
    {
      "name": "candidate_runtime_file_contract",
      "status": "pass",
      "source": "make agent-runtime-smoke",
      "evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json",
      "evidence_sha256": "1111111111111111111111111111111111111111111111111111111111111111"
    },
    {
      "name": "container_provider_lifecycle",
      "status": "pass",
      "source": "make container-provider-smoke",
      "evidence_ref": "artifacts/m4/runtime/container-provider-smoke.json",
      "evidence_sha256": "2222222222222222222222222222222222222222222222222222222222222222"
    },
    {
      "name": "container_provider_timeout_cleanup",
      "status": "pass",
      "source": "make container-provider-timeout-smoke",
      "evidence_ref": "artifacts/m4/runtime/container-provider-timeout-smoke.json",
      "evidence_sha256": "3333333333333333333333333333333333333333333333333333333333333333"
    },
    {
      "name": "container_provider_output_cap",
      "status": "pass",
      "source": "make container-provider-output-cap-smoke",
      "evidence_ref": "artifacts/m4/runtime/container-provider-output-cap-smoke.json",
      "evidence_sha256": "4444444444444444444444444444444444444444444444444444444444444444"
    },
    {
      "name": "egress_allowdeny",
      "status": "pass",
      "source": "make egress-allowdeny-smoke",
      "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json",
      "evidence_sha256": "5555555555555555555555555555555555555555555555555555555555555555"
    }
  ],
  "reviewed_cases": [
    {
      "id": "payments-cpu",
      "status": "pass",
      "notes": "direct and sandbox outputs preserve the required evidence refs"
    },
    {
      "id": "checkout-latency",
      "status": "pass",
      "notes": "cascade output remains evidence-bound"
    },
    {
      "id": "billing-errors",
      "status": "pass",
      "notes": "alert-storm output is acceptable for the retained sample"
    }
  ],
  "human_review": {
    "status": "pass",
    "reviewer": "openclarion-maintainer",
    "notes": "sample reports preserve evidence traceability"
  }
}
```

## Decision Rules

The gate is conservative:

- `defer`: baseline audit evidence is incomplete, representative quality sample
  evidence is missing, too small, or does not cover `single_alert`, `cascade`,
  and `alert_storm`, review evidence is incomplete, the selected candidate has
  no matching candidate-evaluation entry, the selected candidate runtime ref is
  missing or does not match the top-level runtime ref, the selected pass
  candidate lacks required runtime-smoke references, the selected runtime
  candidate uses a loopback registry reference, or the comparator shows no
  improved or human-reviewed candidate case.
- `iterate`: runtime smokes or human review fail, or any quality-comparison case
  regresses, any reviewed quality case fails human review, or the selected
  candidate evaluation does not pass.
- `proceed`: baseline audit passes, required runtime smokes pass, representative
  sample evidence is present across all three alert prompt scenarios, human
  review passes for every quality case, selected-candidate evaluation passes,
  selected pass evaluation cites every required runtime-smoke artifact by
  name, and quality comparison has at least one improved or human-reviewed case
  with zero regressions.

`defer` has decision precedence when evidence is incomplete, but the emitted
`reasons` array still includes any detected iterate-class failures. This keeps
retained evidence from hiding a runtime-smoke failure, regression, or failed
case review behind an incomplete-evidence decision.

The default minimum quality sample size is three cases. Operators may raise it
with `--min-cases`; lowering it should be treated as a local exception and
recorded with the evidence packet.

## Validation

```bash
make sandbox-m4-decision-test
```

Manual decision run:

```bash
make sandbox-m4-decision \
  BASELINE_AUDIT=artifacts/sandbox-baseline-audit.json \
  QUALITY_COMPARISON=artifacts/sandbox-quality-comparison.json \
  REVIEW_EVIDENCE=artifacts/sandbox-m4-review-evidence.json
```

For a full evidence directory that freezes all three inputs plus the decision
output, use [sandbox-m4-evidence-packet.md](sandbox-m4-evidence-packet.md).

The test target is part of `make ci` and has a dedicated GitHub Actions job.
The manual decision target is not auto-run in CI because real candidate runtime
evidence and human review are environment-specific.
