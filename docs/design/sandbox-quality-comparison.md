# Sandbox Quality Comparison

OpenClarion remains an intelligent alert analysis product. The M4 sandbox track
adds a way to compare sandbox-augmented alert reports against the M2 direct LLM
baseline without changing product positioning or accepting a runtime framework
by default.

## Scope

`scripts/sandbox_quality_compare` is an offline comparison helper. It reads two
already-generated `SubReport` JSON files:

- `--direct-sub-report`: report produced by the M2 direct LLM path
- `--sandbox-sub-report`: report produced by a sandboxed runtime candidate
- `--manifest`: JSON manifest containing multiple direct/sandbox pairs

Both files are parsed through `reportdraft.ParseSubReport`, which reuses the
production SubReport JSON Schema and local semantic validation. The manifest
rejects duplicate JSON object keys and unknown fields before parsing, and both
SubReport inputs reject duplicate JSON object keys before production schema
validation. Retained evidence therefore cannot rely on Go's default
last-key-wins unmarshalling or stale extra fields. The tool does not call an
LLM, a sandbox, Prometheus, Docker, OpenClaw, Hermes Agent, or any external
service.
The manifest and every direct/sandbox SubReport input must be regular files;
symlinks and other non-regular files are rejected before the helper reads them.

Manifest mode lets a later real quality run collect representative alert
evidence outputs without changing the comparison tool. The manifest carries a
human-readable `sample_basis`; it must be single-line, unpadded, and no more
than 2048 bytes so retained quality artifacts stay stable for review and
diffing. Each case ID must be single-line, unpadded, and no more than 128
bytes. Each case must declare the alert-analysis prompt `scenario`
(`single_alert`, `cascade`, or `alert_storm`) used to produce the pair. Each
case must also declare single-line `required_evidence_refs`, each no more than
120 runes, including at least one canonical `snapshot:<positive-id>`
EvidenceSnapshot reference, and the helper rejects the case unless both the
direct and sandbox SubReports contain every listed evidence reference either in
top-level `evidence_refs` or finding `evidence_id` fields. This binds
comparison pairs to the same frozen evidence context instead of only trusting
filenames. Report paths must be single-line, slash-separated relative paths
under the manifest directory, no more than 512 bytes, must not contain
parent-directory traversal, and the direct/sandbox paths for a case must point
to distinct files. This keeps a retained evidence packet portable and prevents
a single report file from being counted as both sides of a comparison.
Normalized report paths must also be unique across the whole manifest so one
retained report cannot inflate the representative sample size by appearing in
more than one case.

```json
{
  "sample_basis": "single-alert, cascade, and alert-storm samples from the same alert replay window",
  "cases": [
    {
      "id": "payments-cpu",
      "scenario": "single_alert",
      "required_evidence_refs": ["snapshot:11", "alert:cpu"],
      "direct_sub_report": "direct/payments-cpu.json",
      "sandbox_sub_report": "sandbox/payments-cpu.json"
    }
  ]
}
```

Operators can prepare the manifest from an evidence directory instead of
hand-writing report pairs:

```bash
make sandbox-m4-quality-manifest-prepare \
  ROOT=artifacts/m4/quality-sample \
  SAMPLE_BASIS="single-alert, cascade, and alert-storm samples from the same alert replay window" \
  OUT=artifacts/m4/quality-sample/quality-manifest.json
```

The sample root must use this layout:

```text
direct/single_alert/<case-id>.json
direct/cascade/<case-id>.json
direct/alert_storm/<case-id>.json
sandbox/single_alert/<case-id>.json
sandbox/cascade/<case-id>.json
sandbox/alert_storm/<case-id>.json
```

The helper pairs files by scenario and case ID, parses both sides through the
production SubReport parser, requires all three alert-analysis scenarios, and
derives manifest `required_evidence_refs` from refs shared by the direct and
sandbox reports. Each case must share at least one canonical
`snapshot:<positive-id>` ref. It writes only a manifest input; it does not make
the sample representative, run model inference, review report quality, or
accept a runtime candidate. The output path must not already exist.

After the manifest is retained, operators can run the manual quality
comparison and keep the resulting evidence file:

```bash
make sandbox-m4-quality-compare \
  QUALITY_MANIFEST=artifacts/m4/quality-sample/quality-manifest.json \
  OUT=artifacts/m4/quality-sample/quality-comparison.json
```

The target runs manifest mode with `--fail-on-regression` and refuses to
overwrite an existing comparison file.

## Output

The tool emits one JSON object to stdout by default, or writes the same JSON to
`--out <path>` when a retained output file is requested. The output file must
not already exist and its parent directory must be present. The object
contains:

- the validated schema id
- direct and sandbox report metrics
- sandbox-minus-direct deltas
- a conservative recommendation
- `review_required: true`

In manifest mode, the JSON output contains one entry per case plus aggregate
counts for improved, equivalent, regressed, and needs-human-review outcomes.
It also echoes `sample_basis`, emits `scenario_coverage`, and includes each
case's scenario plus `required_evidence_refs` so the M4 decision gate and
packet validator can verify that representative evidence covers the three alert
prompt modes and remains bound to a canonical EvidenceSnapshot before
proceeding.

Metrics are intentionally simple:

- finding count
- recommended action count
- high-priority action count
- unique evidence reference count, combining `evidence_refs` and finding
  `evidence_id`
- confidence rank (`low` = 1, `medium` = 2, `high` = 3)
- severity rank (`info` = 1, `warning` = 2, `critical` = 3)

Recommendations are heuristics:

- `sandbox_candidate_improved`: no structural regression and at least one
  supportedness/detail metric increased
- `sandbox_candidate_regressed`: sandbox has fewer findings, fewer unique
  evidence references, fewer recommended actions, or lower confidence
- `equivalent_metrics`: compared metrics are equal
- `needs_human_review`: only review-context metrics changed, such as severity
  or high-priority action count

`--fail-on-regression` exits non-zero when the recommendation is
`sandbox_candidate_regressed` for single-pair mode, or when any manifest case
regresses in manifest mode.

## Non-Goals

This helper is not a real model-quality benchmark. By itself, it does not
decide whether the sandbox track proceeds, iterates, or stays deferred. The
M4 decision must combine this output with baseline audit evidence, candidate
runtime smoke evidence, and human review through
[sandbox-m4-decision.md](sandbox-m4-decision.md).

The helper exists so those later comparisons have a repeatable, schema-backed
starting point and fail closed on invalid report JSON.

## Validation

```bash
make sandbox-quality-compare-test
```

The target is part of `make ci` and has a dedicated GitHub Actions job.
