# CI Governance

CI is a product quality boundary, not only a build runner. Required checks are
owned by repository scripts or `make` targets so local and remote validation
stay aligned. Gates are introduced progressively to avoid overloading M0.

## Gate Families

| Gate | Purpose |
|------|---------|
| docs hygiene | no non-English governed docs, valid links |
| generated code | OpenAPI, Ent, and frontend type freshness |
| backend tests | Go unit and integration tests |
| frontend tests | typecheck, unit tests, smoke tests |
| security | secret scan, vulnerability scan, dependency audit |
| architecture | provider boundaries, transaction boundaries, no unmanaged goroutines |
| AI quality | golden prompt structural validation, refusal handling |

## Progressive Gate Schedule

Gates are introduced as the relevant code lands. A gate that depends on
non-existent code is a maintenance burden, not a quality boundary.

| Gate | Introduced At | Status | Notes |
|------|---------------|--------|-------|
| docs hygiene (English-only check) | M0 | landed | `make docs-hygiene` |
| ADR index consistency | M0 | landed | `make adr-check` |
| Markdown relative-link validation | M0 | landed | `make links-check` |
| Forbidden imports (Gin/Echo/Fiber/Redis/Mongo/vector) | M0 | landed | `make forbidden-imports`; activates when Go code lands |
| Forbidden `latest` pins (go.mod, package.json) | M0 | landed | `make forbidden-latest` |
| Forbidden oapi-codegen v2 / openapi.compat.yaml | M0 | landed | `make forbidden-oapi-v2` |
| Forbidden SQLite in Go tests | M0 | landed | `make forbidden-sqlite`; activates when tests land |
| DCO sign-off validation | M0 | landed | `ci.yml` job `dco-check` (PR-only) calls `make dco-check`; rejects PR commits without `Signed-off-by:` |
| Workflow / Makefile parity | M0 | landed | `make workflow-parity`; rejects inline shell, undeclared `make` targets, mutable action refs, and missing job permissions/timeouts |
| Go module: `go vet`, `go build`, `go test ./...` | M0 | landed | `make go-checks` (composite of `generate go-vet go-build go-test`); CI job `go-checks` |
| OpenAPI lint (`vacuum lint --fail-severity error`) | M0 | landed | `make openapi-lint`; vacuum is a `go tool` dependency so the gate is hermetic |
| OpenAPI generation freshness (`make generate` no diff) | M0 | landed | `make openapi-fresh` runs `go generate ./api/...` and rejects any working-tree diff in `api/openapi.gen.go` |
| `oapi-codegen-exp` released-version pin check | M0 | landed | covered by `make forbidden-latest` (rejects `latest` in `go.mod` / `package.json`); concrete pin recorded in `go.mod` (`v0.1.0`) and DEPENDENCIES.md |
| Ent generation freshness | M1-PR1 | landed | `make ent-fresh`; runs `go generate ./internal/persistence/ent/...` and rejects any working-tree diff under `internal/persistence/ent/` |
| Atlas migration drift check | M1-PR1 | landed | `make atlas-drift`; copies `internal/persistence/migrations/` into `.atlas-drift-tmp/` and runs `atlas migrate diff drift_check` via the pinned `arigaio/atlas:1.2.0` Docker image; no-op until the first migration is cut. Companion gate `make atlas-smoke` is a manual one-shot acceptance check (not in `make ci`) - see [database/migrations.md](../database/migrations.md) |
| Temporal workflow tests (replay, signal, timer) | M1 | pending | once first workflow lands |
| Provider boundary lint (forbidden imports) | M2 | pending | once provider layer is real |
| LLM golden prompt structural tests | M2 | pending | once LLMProvider exists |
| LLM refusal / truncation handling tests | M2 | pending | covers Structured Outputs failure modes |
| Frontend typecheck and unit tests | M3 | pending | once `web/` lands |
| OpenAPI -> TS type freshness | M3 | pending | enforces frontend contract sync |
| OpenTelemetry trace integration smoke | M3 | pending | observability check |
| Container sandbox security gate (non-root, limits) | M4 | pending | once ContainerProvider lands |
| WebSocket auth handshake test | M5 | pending | once diagnosis room lands |
| Bounded-turn enforcement test | M5 | pending | guards against client-side bypass |
| Audit completeness test | M5 | pending | every lifecycle event logged |

## Current Private-Incubation Gate

The canonical entry point is `make pr` (and `make ci`), defined in the
repository root `Makefile`. GitHub Actions calls the same `make` targets.

```bash
make pr            # full PR validation bundle
make docs-hygiene  # documentation language gate
make forbidden     # forbidden-method bundle (imports / latest / v2 / sqlite)
make adr-check     # ADR index consistency
make links-check   # relative markdown link validation
```

Additional gates land per the schedule above as code is introduced. A new
gate must ship together with the corresponding `make` target and a row in
the schedule table.

## Workflow Policy

- GitHub Actions step `run:` lines must invoke a single repository-owned
  `make <target>` - no inline pipelines, no chained commands, no
  multi-line block scalars (`run: |`). The `workflow-parity` gate
  enforces this and fails CI on violation.
- A new gate is introduced as: (1) a script under `scripts/`, (2) a
  `make` target wiring it, (3) a CI job calling that target. All
  three land in the same PR.
- Generated-code checks must fail on dirty working tree.
- Allowlist files must have owners and expiration criteria.
- A new gate must ship with documentation in this file before being
  merged.
- Gates that block PRs must run within 10 minutes wall-clock for
  M0-M2.
- Third-party GitHub Actions are pinned to a full commit SHA, with
  the human-readable version in a trailing comment
  (e.g. `actions/checkout@<sha> # v6.0.2`). Mutable tags such as
  `@v4` are forbidden. See `docs/design/DEPENDENCIES.md`.
- Every job declares an explicit `permissions:` block (start from
  `contents: read`) and a `timeout-minutes:` value.

## Future Imports from Shepherd Reference

The shepherd-platform reference project has many ready CI assets. Importing
everything now would create empty gates that depend on code OpenClarion has
not yet committed. The list below records what to import **when the
corresponding code lands**, with a one-line description so the import is
deliberate rather than copy-paste. Do not pre-stage these assets; pull each
one from shepherd at the milestone that activates it.

| Asset (shepherd path) | Purpose | Activate at | Notes |
|-----------------------|---------|-------------|-------|
| `.gitleaks.toml` + `make secrets-scan` target | repository secret scanner; PR-blocking on any finding | M0 (with first source PR) | configure allowlist for governed example values only |
| `.golangci.yml` (baseline rules) | Go static analysis baseline | M0 (with Go skeleton) | start with shepherd's baseline minus shepherd-specific custom rules |
| `.custom-gcl.yml` + `tools/shepherd-linter/` style custom analyzer | architecture boundary lint (forbidden cross-layer imports, provider/transaction discipline) | M2 (with provider layer) | OpenClarion writes its own analyzer; reuse shepherd's plugin-loading pattern only |
| `.github/workflows/api-contract-validation.yml` | OpenAPI lint + breaking-change diff + generation freshness | M0 (with `api/openapi.yaml`) | OpenClarion uses oapi-codegen-exp V3; replace shepherd's v2 bridge logic |
| `.github/workflows/frontend-tests.yml` | typecheck, lint, unit test, build for `web/` | M3 (with `web/` skeleton) | mirrors shepherd structure |
| `scripts/run_pr_parallel.sh` | parallel `make pr` job runner for local development speed | M2+ (when sequential `make pr` exceeds ~10 min) | premature on M0/M1 |
| `scripts/check_public_hygiene.sh` | private-marker scanner for public-incubation transition | private-to-public transition | needed only if/when OpenClarion exits private incubation |
| `release-please-config.json` + `.github/workflows/release-please.yml` + `.github/workflows/release.yml` + `docs/RELEASE.md` | conventional-commit-driven release automation | M3-M4 (first public preview) | conventional commit discipline already enforced in `ai-code/.agent/skills/github-workflow/SKILL.md` so adoption is non-disruptive |
| `docs/design/ci/scripts/check_*.go` static analyzers | provider-wiring / SQLite-in-tests / sqlc / SSA boundary checks (Go-based) | M2-M4 (per analyzer's target) | port one analyzer at a time, only when the analyzed code exists in OpenClarion |
| `docs/design/ci/GATE_HARDENING_CHECKLIST.md` | gate maturity scoring framework | M3+ | once OpenClarion has at least 5 gates, evaluate hardening per shepherd's framework |
| `docs/design/ci/vacuum/.vacuum.yaml` ruleset | OpenAPI semantic linter (Go-native, ~10x faster than Spectral, Spectral-rule compatible per shepherd ADR-0029) | M0 (with first OpenAPI spec) | adopt minimal ruleset; expand as the spec grows. **Do not use `docs/design/ci/spectral/`** - it is deprecated upstream |
| `docs/design/ci/allowlists/` discipline | per-gate allowlist with owners and expiry | when any gate first needs an allowlist | create allowlist only with a justified expiry, not blanket exemptions |
| `docs/design/CURRENT_STATE.md` | living snapshot of milestone progress | M1+ | replaces ad-hoc status notes once cross-milestone tracking is needed |
| `docs/design/DEFERRED_FOLLOWUPS.md` | structured backlog of deferred-but-not-forgotten items | M1+ | use shepherd's section template (item, why deferred, trigger to revisit) |

## Future Hardening Roadmap

The M0 gates are intentionally minimal-but-strict: each one already
rejects the most common drift mode it is responsible for. This section
records every additional tightening we considered and **deliberately
deferred** so that it is not lost. Each entry lists: what to add, the
trigger that makes it worth the cost, and a one-line implementation
hint. **Do not implement these yet.** They are written here so a
future PR can cite the exact entry and so we do not re-derive the
same ideas later.

The convention for activating any of these is the standard
"script + `make` target + CI job" three-piece (Workflow Policy)
plus a row in the Schedule table above.

### Workflow / parity gate (`make workflow-parity`)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Reject `runs-on: ubuntu-latest` and any non-pinned runner version | M1 (when adding the second workflow) | extend the awk pass to require `runs-on:` to match a fixed allow-list `^ubuntu-(24\.04|22\.04)$` |
| Require `concurrency:` block on every PR-triggered workflow | M1 | reject PR-triggered workflows whose top level does not declare `concurrency:` with `cancel-in-progress: true` |
| Require explicit `defaults: run: shell: bash` | M1 | parse top-level `defaults:` and reject workflows whose `run:` steps inherit the default shell |
| Reject `permissions:` blocks broader than `contents: read` unless an inline justification comment is present | M2 (when first workflow legitimately needs write scope) | annotate exceptions with `# parity-allow: <reason>` adjacent to the `permissions:` line and check for it |
| Cap `timeout-minutes:` at 15 (M0-M2) and 30 (M3+) | M1 | enforce numeric upper bound in the awk pass |
| Forbid `secrets.*` references in `pull_request`-triggered workflows; require `pull_request_target` opt-in with explicit reviewer policy | first time a gate needs a secret on PR | regex scan for `${{ secrets.` plus trigger introspection |
| Require an inline `# vX.Y.Z` comment after every `@<sha>` action pin (currently strongly encouraged, not enforced) | M1 | regex check that every `uses: ...@<40-hex>` is followed on the same line by `# v\d+\.\d+(\.\d+)?` |
| Forbid duplicate `name:` across workflows (so failing job names are unambiguous in branch protection rules) | M2 | aggregate all `name:` per workflow file and de-duplicate |
| Require every gate workflow file to be named `ci.yml` or `<gate>.yml` and listed in this README | M2 | cross-reference `.github/workflows/*.y*ml` against the Schedule table |

### DCO gate (`make dco-check`)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Require `Signed-off-by:` email to match the commit author email | M1 | parse trailer email; compare to `git show -s --format=%ae` |
| Forbid GitHub `noreply` addresses in author email (project policy decision) | when project decides whether to allow web-edit commits | regex `@users\.noreply\.github\.com$` reject |
| Require GPG / SSH commit signature verification (`git verify-commit`) on PR commits | when contributor base is large enough that DCO trailer alone is insufficient | call `git verify-commit` on each SHA; allowlist maintainers initially |
| Reject commit messages or trailers that contain AI tool branding (`Generated-by:`, `Co-authored-by: Claude`, model names) | M1 | regex scan subject, body, and trailer block; aligned with `ai-code/.agent/skills/github-workflow/SKILL.md` Conventional Commit Discipline |

### Forbidden-imports / architectural boundaries

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Forbid `os/exec` outside `internal/sandbox/` and `cmd/*` | M2 (provider layer lands) | per-package allow-list, mirrors the provider lint pattern |
| Forbid `unsafe` and `reflect` outside designated infra packages | M2 | same allow-list mechanism |
| Forbid `time.Now()` in domain logic; require injected `Clock` interface | M2 | restrict to packages tagged with `// +clock:allowed` build comment, or to `internal/infra/clock/` |
| Forbid `panic()` outside `main` / `init` / explicit `recover` boundary | M2 | static analyzer using `go/ast` |
| Forbid `fmt.Print*` / `log.Println` in non-test, non-CLI code; require structured logger | M2 | analyzer with allow-list for `cmd/` |
| Forbid raw `database/sql.Open` outside the provider layer | M2 | provider-boundary analyzer (already on roadmap, this is the explicit rule) |
| Forbid `net/http.DefaultClient` and bare `http.Get` (require timeout + retry middleware) | M2 | analyzer; aligns with stability principle |
| Forbid Goroutine creation outside `errgroup` / supervised helpers | M3 | SSA-based analyzer; matches shepherd's "no unmanaged goroutine" gate |

### Dependency pinning (`make forbidden-latest` family)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Require concrete `module@version` pins for first-import-tracked Go modules (Temporal SDK, Ent, Atlas, OTel); reject `replace` directives for these without an entry in DEPENDENCIES.md | M1 (when these modules first land in `go.mod`) | scan `go.mod` for known module paths; require a non-`latest` version; cross-reference DEPENDENCIES.md allow-list. The `oapi-codegen-exp` `v0.1.0` pin (M0) is the prototype pattern. |
| Reject indirect dependencies older than N months without an `indirect` allow-list | M2 | query Go module proxy or OSV metadata for module release dates; cross-reference an expiry-based allow-list |
| Reject `replace` directives pointing at forks not listed in `docs/design/DEPENDENCIES.md` allow-list | M1 | parse `go.mod` `replace` block; cross-reference allow-list |
| Reject `package.json` entries with `^` or `~` ranges for first-party packages | M3 (with `web/`) | regex scan; allow ranges only for transitive dev tooling |
| Reject Docker base images without a digest pin (`@sha256:...`) | M4 (with first Dockerfile) | grep `FROM` lines |

### ADR governance gate (`make adr-check`)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| YAML front-matter schema validation (`id`, `title`, `status`, `date`, optional `supersedes`, `superseded_by`) | M1 | small Go or `yq` script; reject unknown keys |
| Cross-reference closure: `supersedes: ADR-X` requires ADR-X exists with `status: superseded` and a matching `superseded_by` back-pointer | M1 | second pass over the parsed metadata |
| Reject ADR PRs that change a previously-`accepted` ADR's body (must be done via a new superseding ADR) | M1 | git-diff check on ADR files when status was `accepted` at base |
| Enforce 48-hour governance window via PR labels rather than honor system | M2 (when external contributors arrive) | bot adds `adr-window-start` timestamp label; merge gate checks elapsed time |
| Reject ADR files without a one-paragraph "Consequences" section | M1 | section-header presence check |

### Markdown / docs hygiene (`make links-check`, `make docs-hygiene`)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Validate intra-document anchor links (`#section`) point at existing headings | M1 | extend `check_markdown_links.sh` with a heading-table pass |
| External `https://` link liveness check (cached, weekly) | M3 | separate scheduled workflow, never PR-blocking; surface as report |
| Markdown lint (heading-level jumps, list indentation, line length) on governed docs | M1 | `markdownlint-cli2` pinned to a SHA; ruleset in `docs/design/ci/markdownlint.yaml` |
| Project terminology / wordlist enforcement (e.g. "MCP-over-Streamable-HTTP" single canonical spelling) | M1 | wordlist file + grep gate |
| Reject orphan documents (files under `docs/` not reachable from `docs/README.md`) | M2 | reachability traversal from a root index |

### Code generation freshness

| Tightening | Trigger | Implementation hint |
|---|---|---|
| `make generate` must produce zero git diff (already on Schedule) | M0 with first generator | run `make generate && git diff --exit-code` in CI |
| Generated files must carry a `// Code generated ... DO NOT EDIT.` header; reject hand edits | M0 | regex check on known generated paths |
| Generation idempotence: running `make generate` twice yields identical output | M1 | second run + diff in CI |
| Generator binaries pinned via the Go 1.24+ `tool` directive in `go.mod` (e.g. `oapi-codegen-exp`, `vacuum`); reject `latest` for any tool entry | M0 (landed implicitly via `forbidden-latest` + the `tool (...)` block in `go.mod`) | combine with the dependency-pinning rule above |

### Supply-chain & secrets

| Tightening | Trigger | Implementation hint |
|---|---|---|
| `gitleaks` PR-blocking secrets scan (already in Future Imports table) | M0 (with first source PR) | adopt shepherd's `.gitleaks.toml`; allowlist only governed example values |
| `govulncheck` on every PR (Go) | M0 (with Go skeleton) | `govulncheck ./...` via a `make vuln-check` target |
| `osv-scanner` for `package.json` and lockfiles | M3 | mirrors the Go path |
| `go-licenses` license compliance gate (reject GPL / AGPL etc.) | M2 | allow-list of acceptable SPDX IDs in `docs/design/DEPENDENCIES.md` |
| SBOM generation (CycloneDX) attached to every release artifact | M3-M4 | `cyclonedx-gomod` for Go, `@cyclonedx/cdxgen` for npm; integrate into release-please pipeline |
| Container image vulnerability scan (Trivy / grype) on push | M4 | runs against the digest pinned in the build pipeline |
| Sigstore / cosign artifact signing | M4 | optional but recommended once we ship containers |

### Test discipline

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Coverage non-regression on PR (per-package thresholds) | M2 (after enough tests exist that a baseline is meaningful) | `go test -coverprofile`; compare against base branch |
| Integration tests must use Testcontainers (forbidden SQLite gate already enforces the negative half) | M1 | analyzer scans `*_test.go` for `testcontainers-go` import in any package that touches `database/sql` |
| Integration tests must not depend on host network (no outbound DNS resolves to public ranges) | M2 | sandboxed runner with egress block; failure mode is documented in the test contract |
| Race detector must be on for `go test` in CI | M0 (with Go skeleton) | `go test -race ./...` in the `make go-test` target |
| Benchmark regression gate (>10% slowdown blocks PR) | M3 | `benchstat` comparison; soft-failure first, hard-failure once stable |
| Prompt golden tests structural validation (already on Schedule) | M2 | listed for completeness |

### Performance & cost budgets

| Tightening | Trigger | Implementation hint |
|---|---|---|
| `make pr` total wall-clock budget: 10 min (M0-M2), 15 min (M3+) | always | timed wrapper in CI; soft warn, then enforce |
| Per-job timeout cap (see workflow-parity hardening above) | M1 | numeric bound in awk pass |
| Workflow run-time alerting (median + p95 dashboard) | M3 | scheduled report job; not PR-blocking |
| CI artifact size cap (per artifact, total per PR) | M3 | post-job inventory; surfaces accidental binary commits |
| Cache hit rate floor for module / npm caches | M3 | observability only; informs cache key tuning |

### PR / commit governance

| Tightening | Trigger | Implementation hint |
|---|---|---|
| PR title matches Conventional Commit (already mandated in skill, not yet machine-checked) | M1 | small script or pinned action validates `pull_request.title`; if Node tooling is used, pin it in the lockfile |
| PR description must contain non-empty `## Risk` and `## Rollback` sections | M2 | regex check on `pull_request.body` |
| PR file count cap (default: 50; opt-out with maintainer label) | M2 | API call from a dedicated workflow |
| PR must link an issue or ADR for any change touching `docs/adr/` or `internal/sandbox/` | M2 | regex on `pull_request.body` plus path-filter |
| Single-PR-per-workflow-file rule for `.github/workflows/*` changes | M2 | path-filter check; forces explicit reviewer attention on CI changes |
| Squash-merge enforced as the only allowed merge strategy | M1 | repo setting + a guard workflow that warns on merge-commit shape |
| Branch protection: require all M0 jobs as required checks | M1 | configured in repo settings, recorded here as the policy source |
| Negative-test fixture per gate (`tests/ci/negative/<gate>.sh`) committed alongside the gate | M2 | repository convention; the existing `parity_neg_v2.sh` is the prototype |

### Documentation of these rules

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Move each activated row out of this section and into the Schedule table | continuous | each PR that lands a hardening item also deletes its row here |
| Annotate every gate with an "escape-hatch owner + expiry" when allowlist files appear | first allowlist file | enforced by the existing Workflow Policy bullet on allowlists |
| Keep `make pr` as the primary reproducible local CI command, with explicit documentation for PR-context-only exceptions | always | DCO is the current exception because its authoritative range exists only on `pull_request`; record any new deviation as a deliberate ADR |
