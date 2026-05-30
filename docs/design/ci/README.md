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
| docs hygiene (English-only, terminology, and proof-state check) | M0 -> M1 hardening | landed | `make docs-hygiene`; rejects non-English CJK literals, checks `docs/design/ci/terminology.tsv` so governed docs keep OpenClarion positioned as intelligent alert analysis, rejects generic platform positioning, restricts `SignalEvent` / `SignalGroup` / `CaseGroup` aliases to architecture boundary docs, and verifies `docs/design/END_TO_END_VERIFICATION.md` only uses verdicts defined in its Verdict Scale table. |
| ADR index, front matter, Consequences, supersession, and immutability validation | M0 -> M1 hardening | landed | `make adr-check`; validates ADR index/file consistency, monotonic IDs, schema-valid ADR front matter (`id`, `title`, `status`, `date`, participants), accepted status values, file/H1/front-matter identity consistency, a non-empty `Consequences` section in every ADR, `supersedes` / `superseded_by` cross-reference closure, and PR/base-aware accepted ADR body immutability via `ADR_BASE_REF`. `go test ./scripts` carries fixture-backed positive and negative front-matter / Consequences / supersession / immutability tests. |
| Markdown link, anchor, and docs reachability validation | M0 -> M1 hardening | landed | `make links-check`; validates relative Markdown target paths plus same-file and cross-file Markdown `#anchor` fragments against generated heading slugs or explicit HTML anchors, and rejects Markdown files under `docs/` that are not reachable from `docs/README.md` through relative Markdown links. `go test ./scripts` carries fixture-backed positive and negative anchor and orphan-document tests. |
| External HTTPS link inventory | M3 docs hardening | landed / manual | `make external-links-check`; inventories governed Markdown external HTTP(S) links locally without network access. Live HEAD-with-GET-fallback liveness is intentionally deferred to an isolated workflow PR so workflow-file review remains focused. |
| Markdown structure lint | M1 hardening | landed | `make markdownlint`; runs pinned `markdownlint-cli2` from `web/package-lock.json` with `docs/design/ci/markdownlint/.markdownlint-cli2.jsonc`, enforcing heading-level increments, unordered-list indentation, and prose line length for governed Markdown. `go test ./scripts` verifies the wrapper uses the local pinned binary and fails fast when dependencies are not installed. |
| Forbidden imports (Gin/Echo/Fiber/Redis/Mongo/vector) | M0 -> W3-2b | replaced | Legacy `scripts/check_no_forbidden_imports.sh` retired after analyzer equivalence verification; covered by `openclarion-arch` in `make go-lint` |
| Forbidden mutable dependency pins (go.mod, package.json, Dockerfile) | M0 -> M4 hardening | landed | `make forbidden-latest`; rejects `latest` in Go/npm manifests, requires critical first-import Go modules (Ent, Temporal SDK, OTel) to stay direct concrete root-module pins without `replace`, requires Go `tool` directive paths to resolve to concrete `require` version pins, rejects undocumented Go `replace` directives unless `docs/design/DEPENDENCIES.md` carries a matching `replace-allow: <module> => <target>` marker with owner and expiry, rejects `^` / `~` direct dependency ranges in first-party `package.json` files once `web/` lands, and rejects external Dockerfile base images without an immutable `@sha256:` digest while allowing `scratch` and previous build stages. `go test ./scripts` carries Go module, Go tool directive, replace allowlist, Dockerfile, and npm positive/negative fixtures. |
| Forbidden oapi-codegen v2 / openapi.compat.yaml | M0 | landed | `make forbidden-oapi-v2` |
| Forbidden SQLite in Go tests | M0 | landed | `make forbidden-sqlite`; activates when tests land |
| PR title Conventional Commit validation | M1 hardening | landed | `ci.yml` job `pr-title-check` (PR-only) calls `make pr-title-check`; validates `github.event.pull_request.title` against the Conventional Commits header shape `type(scope)!: description` with optional scope and breaking-change marker. `go test ./scripts` carries positive and negative title fixtures. |
| PR description risk/rollback validation | M2 hardening | landed | `ci.yml` job `pr-description-check` (PR-only) calls `make pr-description-check`; reads `pull_request.body` from `GITHUB_EVENT_PATH` and requires non-empty `## Risk` and `## Rollback` sections. `go test ./scripts` covers env/local mode, GitHub event JSON mode, missing sections, empty/comment-only sections, empty bodies, and fenced-code false positives. |
| PR impact reference validation | M3 hardening | landed / PR-only | `ci.yml` job `pr-impact-reference-check` calls `make pr-impact-reference-check`; if a PR changes `docs/adr/` or future `internal/sandbox/` paths, the PR body must link an issue or ADR using `#123`, a GitHub issue/PR URL, `ADR-0001`, or a concrete `docs/adr/ADR-0001-*.md` path. `make pr-impact-reference-check-test` covers path triggers, issue/ADR reference forms, event JSON body loading, PR diff-range fetching, missing env pairs, and no-impact bypass. |
| DCO sign-off validation | M0 -> M1 hardening | landed | `ci.yml` job `dco-check` (PR-only) calls `make dco-check`; rejects PR commits without `Signed-off-by:`, rejects sign-off emails that do not match the commit author email, and rejects commit messages/trailers with AI tool branding such as `Generated-by:`, AI co-author trailers, or model names. `go test ./scripts` carries temporary-git-repository fixtures for missing, mismatched, and AI-branded commits. |
| Workflow / Makefile parity | M0 -> M2 hardening | landed | `make workflow-parity`; rejects inline shell, undeclared `make` targets, mutable action refs, missing action version comments, missing PR concurrency, `pull_request` secret references, `pull_request_target` workflows without an explicit reviewer-policy marker, missing default bash shell, unpinned runners, job permissions broader than `contents: read` without `parity-allow`, missing job permissions/timeouts, M0-M2 timeouts above 15 minutes, unregistered workflow files, and duplicate workflow names. `go test ./scripts` carries fixture-backed negative tests for workflow registry, permission drift, and PR secrets boundaries. |
| Go module: generate, vet, build, tests | M0 -> M3 | landed | `make go-checks` (composite of `generate go-vet go-build go-test`) runs the root-module Go package set under `api/`, `cmd/`, `internal/`, and `scripts/`; `go-test` uses `go test -race -count=1`; CI job `go-checks` |
| Go coverage floor | M3 hardening | landed | `make go-coverage`; runs `scripts/check_go_coverage.sh`, excludes generated API/Ent packages and root script-test aggregators, and requires every selected handwritten Go package with statements to stay at or above `GO_COVERAGE_MIN` (default 40.0%). Root `scripts` tests prove package filtering, threshold failures, and invalid threshold handling. |
| Testcontainers integration-test contract | M1 -> M3 hardening | landed | `make testcontainers-contract`; parses real Go `_test.go` files outside `testdata/` with `go/parser` / `go/ast`, groups imports by test package directory, rejects any package that imports `database/sql` without also importing `github.com/testcontainers/testcontainers-go...`, and rejects direct host/public network entry points such as `net/http.Get`, `net/http.DefaultClient`, and `net.Dial` in tests. `go test ./scripts` covers same-file harnesses, split setup files, missing Testcontainers imports, analyzer fixture exclusions, commented/string import false positives, allowed `httptest` + injected-client usage, and direct-network negative fixtures. |
| Generated file header validation | M0 -> M1 hardening | landed | `make generated-headers`; validates committed OpenAPI Go, Ent, and frontend OpenAPI TypeScript generated files retain their generator-owned headers / direct-edit warnings. `go test ./scripts` carries positive and negative fixtures for each generated family. |
| OpenAPI lint (`vacuum lint --fail-severity error`) | M0 | landed | `make openapi-lint`; vacuum is a `go tool` dependency so the gate is hermetic; ruleset lives at `docs/design/ci/vacuum/.vacuum.yaml` and explicitly accepts OpenClarion's snake_case JSON convention |
| OpenAPI generation freshness (`make generate` no diff) | M0 | landed | `make openapi-fresh` snapshots `api/openapi.gen.go`, runs `go generate ./api/...`, and rejects generator-induced diffs |
| Repository generation freshness and idempotence | M1 -> M3 hardening | landed | `make generate-fresh`; snapshots tracked plus non-ignored untracked repository files, runs `make generate`, rejects any generator-induced file diff, then runs `make generate` a second time and rejects non-deterministic second-run output. `go test ./scripts` carries stable, tracked-diff, untracked-diff, and second-run-diff fixtures. |
| `oapi-codegen-exp` released-version pin check | M0 | landed | covered by `make forbidden-latest` (rejects `latest` in `go.mod` / `package.json`); concrete pin recorded in `go.mod` (`v0.1.0`) and DEPENDENCIES.md |
| Ent generation freshness | M1-PR1 -> M3 | landed | `make ent-fresh`; snapshots `internal/persistence/ent/`, runs `make ent-generate`, and rejects generator-induced differences while allowing intentional in-progress Ent changes that are already generated |
| Atlas migration drift check | M1-PR1 | landed | `make atlas-drift`; copies `internal/persistence/migrations/` into `.atlas-drift-tmp/` and runs `atlas migrate diff drift_check` via the pinned `arigaio/atlas:1.2.0` Docker image; no-op until the first migration is cut. Companion gate `make atlas-smoke` is a manual one-shot acceptance check (not in `make ci`) - see [database/migrations.md](../database/migrations.md) |
| Secret scanning (gitleaks) | W1-1 | landed | `make secrets-scan`; runs pinned gitleaks with `.gitleaks.toml` config over git history plus a current-source snapshot of tracked and untracked, non-ignored files; CI job `secrets-scan` |
| Go vulnerability scan (govulncheck) | W1-2 -> M3 | landed | `make govulncheck`; runs `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4` over the root-module Go package set and `./...` inside `tools/openclarion-linter`; CI job `govulncheck` |
| Go license compliance | M2 supply-chain hardening | landed | `make go-licenses-check`; runs pinned `go-licenses v1.6.0` with `--include_tests` over the root Go package set and `tools/openclarion-linter`, ignores first-party package prefixes while still checking their dependencies, and accepts only the SPDX IDs listed by `go-license-allow:` in [DEPENDENCIES.md](../DEPENDENCIES.md). `go test ./scripts` covers allowlist parsing, command wiring, missing policy markers, and tool failure propagation. |
| OSV npm lockfile scan | M3 supply-chain hardening | landed | `make osv-scan`; runs pinned `osv-scanner v1.9.2` against committed `package-lock.json` files outside `node_modules`, fails if first-party `package.json` files exist without npm lockfiles, and scans `web/package-lock.json` in CI. `go test ./scripts` covers lockfile discovery, node_modules exclusions, missing lockfiles, and tool failure propagation. |
| Go lint baseline (golangci-lint) | W1-3 | landed | `make go-lint`; bootstraps `github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2` into ignored `bin/`; CI job `go-lint` |
| Dependency update policy (Dependabot) | W2-1 -> M3 | landed | `.github/dependabot.yml`; weekly GitHub Actions, Go module, and `/web` npm version checks, patch/security grouping, no blanket ignores for Temporal/Ent/Atlas/OTel/Next; linter-submodule `golang.org/x/tools` minor/major version updates are intentionally coordinated with the pinned `golangci-lint` binary because `make go-lint` enforces exact plugin parity |
| Custom analyzer scaffold + version parity | W3-1 -> M3 | landed | `make go-lint` now runs `tools/openclarion-linter` tests, validates `golang.org/x/tools` parity via `scripts/check_lint_version.sh`, builds `.custom-gcl.yml` with `golangci-lint custom`, and runs `bin/custom-gcl` over the root-module Go package set |
| Provider/domain forbidden-import analyzer | W3-2a | landed | `openclarion-arch` analyzer enforces production domain/usecase imports and concrete-provider bans |
| Legacy forbidden-import bash deletion | W3-2b | landed | Retired `scripts/check_no_forbidden_imports.sh` after `analysistest` red/green fixtures proved every retired legacy deny-list prefix plus provider-boundary coverage, with a retained deny-list drift test; `make forbidden` now only runs non-Go-plugin forbidden gates |
| Ent transaction boundary analyzer | W3-3 | landed | `openclarion-arch` detects `ent.Client.Tx`, `ent.Tx.Commit`, and `ent.Tx.Rollback` calls outside generated Ent and repository packages using `go/types` |
| Runtime fake-provider analyzer | W3-4 -> M2 hardening | landed | `openclarion-arch` rejects production `cmd/openclarion` imports of any `internal/providers/*/fake` package; test files remain exempt |
| Structured logging analyzer | W3-5 -> M2 hardening | landed | `openclarion-arch` rejects direct `fmt.Print*` and standard-library `log.Print*` / `log.Fatal*` / `log.Panic*` calls in non-test production packages, while allowing CLI packages under `cmd/`, repository scripts under `scripts/`, and generated Ent code. Production diagnostics should flow through structured logging / explicit response writers instead of ad-hoc stdout/stderr output. |
| HTTP client boundary analyzer | W3-6 -> M2 hardening | landed | `openclarion-arch` rejects `http.DefaultClient` and bare `http.Get` / `http.Head` / `http.Post` / `http.PostForm` in non-test production packages, while allowing CLI packages under `cmd/` and repository scripts under `scripts/`. Production outbound HTTP must use request-scoped contexts and injected clients so timeout, tracing, and request-id propagation remain explicit. |
| SQL open boundary analyzer | W3-7 -> M2 hardening | landed | `openclarion-arch` rejects direct `database/sql.Open` calls outside the persistence repository / generated Ent boundary. Usecase, provider, transport, and runtime wiring code must receive persistence factories or repositories instead of opening database connections directly. |
| Process execution boundary analyzer | W3-8 -> M4 hardening | landed | `openclarion-arch` rejects direct `os/exec.Command` / `os/exec.CommandContext` calls outside `cmd/`, repository `scripts/`, and the sandbox boundary. Production alert-analysis code should reach external tools through Provider interfaces or the audited sandbox runtime, not ad-hoc subprocess launches. |
| Core filesystem boundary analyzer | W3-15 -> M2 hardening | landed | `openclarion-arch` rejects direct local filesystem reads, metadata/directory probes, writes, creates, removes, renames, links, temp paths, and local filesystem adapters in non-test `internal/domain` and `internal/usecases` packages. Core alert-analysis logic must receive file-backed evidence or configuration through providers, repositories, or boundary-layer DTOs instead of opening local paths directly. |
| Core clock boundary analyzer | W3-9 -> M2 hardening | landed | `openclarion-arch` rejects direct `time.Now()` calls in non-test `internal/domain` and `internal/usecases` packages. Core alert-analysis logic must receive timestamps from input DTOs, workflow/activity boundaries, or an injected clock so replay and unit tests stay deterministic. |
| Core context boundary analyzer | W3-13 -> M2 hardening | landed | `openclarion-arch` rejects `context.Background()`, `context.TODO()`, and `context.WithoutCancel()` in non-test `internal/domain` and `internal/usecases` packages. Core alert-analysis logic must receive cancellation and deadlines from callers or boundary orchestration instead of creating root or detached contexts internally. |
| Core environment/config boundary analyzer | W3-14 -> M2 hardening | landed | `openclarion-arch` rejects direct `os.Getenv`, `os.LookupEnv`, `os.Environ`, `os.ExpandEnv`, `os.Setenv`, `os.Unsetenv`, `os.Clearenv`, and `flag.*` process flag parsing calls in non-test `internal/domain` and `internal/usecases` packages. Core alert-analysis logic must receive configuration from boundary wiring instead of reading or mutating process environment directly. |
| Reflection / unsafe import boundary analyzer | W3-10 -> M2 hardening | landed | `openclarion-arch` rejects `reflect` and `unsafe` imports in handwritten production files, while allowing tests and generated code identified by Go's generated-code marker. Reflection and unsafe escape hatches must stay in generated code or a deliberately reviewed infra boundary. |
| Panic boundary analyzer | W3-11 -> M2 hardening | landed | `openclarion-arch` rejects `panic()` in handwritten production files outside `main`, `init`, or a deferred function literal that directly calls `recover()`. Core alert-analysis code must return errors instead of relying on crash semantics; transaction wrappers may still re-panic after rollback inside explicit recover boundaries. |
| Goroutine supervision boundary analyzer | W3-12 -> M3 hardening | landed | `openclarion-arch` rejects raw `go` statements in handwritten production files outside `internal/supervision`. Long-running work must use `errgroup.Group.Go` or an approved supervision helper so cancellation and error propagation stay explicit. |
| Core mutable global state analyzer | W3-16 -> M2 hardening | landed | `openclarion-arch` rejects package-level mutable collection state in non-test handwritten `internal/domain` and `internal/usecases` packages, including direct collection variables plus pointer/interface-backed collection initializers. Core alert-analysis logic must keep reusable schemas and lookup tables as constants or pure functions so later calls cannot be polluted by process-global map, slice, array, or channel state. |
| OpenAPI breaking-change diff | W4-1 | landed / soft-fail until 2026-06-10 | `make openapi-breaking`; oasdiff `v1.11.7`; soft-fail owner: CI maintainers; after 2026-06-10 the script hard-fails automatically. Root `scripts` tests cover pre-sunset soft-fail, on/after-sunset hard-fail, non-breaking success, invalid sunset config, and invalid current-date config. |
| OpenAPI critical-node fingerprint lock | W4-2 -> M3/M5 | landed | `make openapi-fingerprint`; lock file `docs/design/ci/locks/openapi-critical.lock` generated from the real `api/openapi.yaml` critical domain API nodes, now including dashboard, alert, evidence, report read paths, report replay trigger path, diagnosis room creation, and diagnosis WebSocket ticket issuance. The lock parser rejects duplicate-key or trailing-value JSON before comparing fingerprints. |
| Documentation shipped-claims consistency | W4-3 | landed | `make doc-claims-check`; checks path hints in `docs/design/CURRENT_STATE.md` shipped rows exist on disk |
| Gate hardening maturity checklist | M3 hardening | landed | `make gate-hardening-check`; validates [GATE_HARDENING_CHECKLIST.md](GATE_HARDENING_CHECKLIST.md) covers every activated Progressive Gate Schedule row exactly once with an allowed maturity level, concrete evidence, and a concrete next-hardening note |
| Go toolchain version consistency | M3 hardening | landed | `make go-toolchain-check`; verifies every first-party `go.mod` uses the root patch-pinned Go directive, parses `.golangci.yml` and workflow YAML as single-document duplicate-key-free config before reading gate-owned version fields, keeps `.golangci.yml` on the matching major.minor language version, and requires every `actions/setup-go` step to read `go-version-file: go.mod` instead of a hard-coded version |
| Allowlist discipline | M3 hardening | landed | `make allowlist-discipline`; validates repository allowlist entries carry adjacent `Owner`, `Expires`, and `Removal trigger` comments so false-positive suppressions remain reviewable and temporary. `make allowlist-discipline-test` covers missing metadata, detached metadata, expired entries, and read-error handling. |
| Workflow change isolation | M3 hardening | landed / PR-only | `make workflow-change-guard`; PR-only CI job rejects pull requests that change more than one `.github/workflows/*.yml` / `.yaml` file. Companion script/docs/Makefile changes may land with the single workflow file so the standard gate-introduction three-piece remains possible, but multi-workflow edits must be split for focused review. `make workflow-change-guard-test` covers path normalization, duplicate handling, CI diff-range fetching, local fallback, and violation reporting. |
| PR wall-clock budget | M3 hardening | landed | `make pr` runs `scripts/pr_budget` around `make ci` with `PR_BUDGET` defaulting to 15 minutes and `PR_BUDGET_MODE=enforce`; enforced runs terminate the child command on timeout, while `PR_BUDGET_MODE=warn` is available for temporary local diagnostics. CI runs `make pr-budget-test` so the wrapper's parsing, exit-code preservation, warn mode, timeout termination, and over-budget enforcement stay covered without recursively running the whole PR bundle inside CI. |
| Temporal workflow tests (snapshot-bound start + replay + signal completion) | M1 | landed | `make temporal-workflow-tests`; focused CI job runs `internal/orchestrator/temporal` integration tests with race detector, verifies the workflow starts from an `EvidenceSnapshot`-bound task, and replays a completed workflow history via `worker.NewWorkflowReplayer` |
| LLM golden prompt structural tests | M2 | landed | `go test ./internal/usecases/reportprompt`; covers single-alert, cascade, alert-storm, and FinalReport request shape, JSON-only fallback instructions, schema selection, idempotency-key shape, invalid input rejection, and schema-compatible fixture outputs without calling a real provider |
| LLM output acceptance tests | M2 | landed | `go test ./internal/usecases/llmoutput`; covers refusal, truncation / non-`stop` finish reasons, invalid JSON, JSON Schema violations, unsupported output modes, and `json_object` fallback validation |
| LLM validation retry tests | M2 | landed | `go test ./internal/usecases/llmretry`; covers validation-feedback retries, max-attempt exhaustion, non-retryable refusal, provider errors, request validation, and context cancellation |
| LLM report draft schema tests | M2 | landed | `go test ./internal/usecases/reportdraft`; covers SubReport/FinalReport valid parse, missing/extra/invalid enum rejection, local semantic bounds, defensive schema copies, and OpenAI strict structured-output compatibility (`additionalProperties:false`, all properties required, root object schemas, approved keyword subset only) |
| OpenAI-compatible LLM provider tests | M2 | landed | `go test ./internal/providers/llm/openai`; covers Chat Completions request shape for strict `json_schema`, `json_object` fallback request shape, refusal mapping, API error wrapping, ambiguous/oversized response-envelope rejection, plain-text API errors, invalid request preflight, strict capability probe success, strict-unsupported fallback, ambiguous capability-error rejection, and non-capability probe error propagation |
| Report persistence repository tests | M2 | landed | `go test ./internal/domain ./internal/persistence/repository`; covers SubReport/FinalReport constructors, per-snapshot SubReport idempotency, global FinalReport idempotency, FinalReport-to-SubReport fan-in links, notification delivery pending/delivered/failed persistence, newest-first list queries, and repository invariant errors |
| Report Temporal workflow tests | M2 | landed | `go test ./internal/orchestrator/temporal`; covers ReportFanOutWorkflow/ReportBatchWorkflow/FinalReportWorkflow scheduling, batch parent-child fan-out/fan-in, GenerateSubReport/GenerateFinalReport persistence and idempotency with a fake LLM provider, and SendReportNotification delivery-log persistence / duplicate delivered skip behaviour before and after IMProvider calls |
| Report trigger tests | M2 | landed | `go test ./internal/usecases/alertreplay ./internal/usecases/reporttrigger ./internal/orchestrator/temporal ./internal/transport/http ./cmd/openclarion`; covers `ReplayWindowForReport` snapshot refs for saved and duplicate snapshots, replay-to-ReportBatchWorkflow start request mapping, Temporal client start options / idempotent duplicate policies, HTTP trigger request/response mapping, CLI one-shot flag parsing / request mapping / JSON output / wait-result mapping, unconfigured 503 handling, and env-driven Prometheus trigger wiring |
| IM provider tests | M2/M5 | landed | `go test ./internal/providers/im/...`; covers deterministic fake provider scripts/copy isolation and Webhook JSON POST shape, idempotency headers, FinalReport and DiagnosisTask notification subjects, optional Bearer auth, response parsing, ambiguous non-object/duplicate/trailing response rejection, oversized response rejection, retryable status classification, and invalid input/config handling |
| Report HTTP API tests | M2 | landed | `go test ./internal/transport/http`; covers `GET /api/v1/reports`, `GET /api/v1/reports/{report_id}`, generated response mapping for linked SubReports with `evidence_snapshot_id`, not-found handling, and shared bounded-list validation |
| Runtime provider wiring tests | M2/M5 | landed | `go test ./cmd/openclarion`; covers env-driven OpenAI-compatible LLM provider injection, Webhook IM provider injection, Prometheus trigger wiring, diagnosis-room OIDC/ticket/Temporal relay/starter wiring, Docker-backed diagnosis sandbox provider wiring, browser origin/CORS allowlist parsing, CLI one-shot argument validation, unconfigured startup compatibility, and partial-config fail-fast behavior |
| Report local E2E | M2 | landed | `go test ./internal/e2e`; covers generated HTTP replay trigger -> Prometheus-compatible alert provider -> alert replay/group/snapshot persistence -> real Temporal ReportBatchWorkflow worker -> OpenAI-compatible provider -> Webhook provider -> persisted FinalReport/SubReport/ReportNotificationDelivery rows |
| Report live smoke harness | M2 | manual | `make report-live-smoke`; not run in CI because it requires real `DATABASE_URL`, `TEMPORAL_HOST_PORT`, `OPENCLARION_PROMETHEUS_URL`, a report-capable worker, and an alert window. Runs `openclarion report-replay --wait` and validates the JSON proof with `scripts/report_live_smoke_output` |
| Report live proof validator | M2 | landed | `make report-live-smoke-output-test`; validates `scripts/report_live_smoke_output`, including regular-file/no-symlink proof input checks before reading, duplicate JSON object key and unknown-field rejection, a canonical UTC and non-future `checked_at`, replay request metadata (canonical UTC window timestamps, valid window, positive limit, supported scenario, explicit wait intent, positive wait timeout), bounded whitespace-free workflow/run IDs with workflow ID binding when explicitly requested, replay stats, zero failed ingest/replay counts, `events_loaded <= request.limit`, group outcome counts matching `groups_built`, snapshot saved/duplicate counts matching retained snapshot refs, snapshot event counts summing to `events_loaded`, non-empty uniquely identified snapshots with contiguous group indexes, one unique SubReport per snapshot, terminal workflow result metadata, a bounded matching `notification_idempotency_key`, bounded single-line provider message ID formatting when supplied by the upstream, and a successful notification status (`accepted` or `delivered`) before a retained `make report-live-smoke` artifact can support live E2E claims. |
| Prometheus MetricsProvider tests | M1/M2 | landed | `go test ./internal/providers/metrics/prometheus`; covers Prometheus alert envelope decoding, strict parsed-envelope rejection before `client_golang` parsing for 2xx success and 400/422 API error responses, oversized parsed-response rejection, firing-state filtering, Bearer auth injection, and upstream non-2xx / malformed-JSON error wrapping |
| Prometheus metrics endpoint tests | M3 | landed | `go test ./internal/observability/metrics`; covers isolated registry construction, `/metrics` scrape output, low-cardinality API request counter, duration histogram, and in-flight gauge instrumentation |
| OpenTelemetry HTTP + Temporal tracing tests | M3 | landed | `go test ./internal/observability/tracing`; covers env-driven OTLP enablement, disabled no-op default, unsupported exporter rejection, W3C propagator wiring, low-cardinality ServeMux route-pattern span names, outbound HTTP `traceparent` + `X-Request-ID` propagation with an in-memory SDK exporter, an OTLP HTTP collector smoke that receives exported spans, and Temporal workflow span propagation through the Temporal OTel interceptor |
| HTTP correlation / access logging tests | M3 | landed | `go test ./internal/observability/correlation ./internal/observability/accesslog ./internal/transport/http ./internal/providers/metrics/prometheus ./internal/providers/llm/openai ./internal/providers/im/webhook`; covers `X-Request-ID` preservation/generation, unsafe ID rejection, outbound request-id propagation, Prometheus endpoint userinfo rejection before outbound client construction, request/trace/span log attributes, structured access logs without raw target/query leakage, and HTTP error logging through context-aware `slog.LogAttrs` helpers |
| Frontend typecheck / lint / unit / build / deadcode / audit | M3 | landed | `make frontend-checks`; runs `npm ci`, `tsc --noEmit`, ESLint, Vitest, `next build`, Knip, and `npm audit --audit-level=high` for `web/` |
| Frontend Playwright route smoke | M3/M5 | landed | `make ci-frontend-smoke`; runs Next.js production server plus a mocked OpenClarion API/WebSocket endpoint and verifies `/dashboard`, `/reports`, and `/reports/{report_id}` render generated-contract-shaped fixture data. It also verifies `/diagnosis-room` can issue a diagnosis WebSocket ticket, receive `ready` and `state` frames, and complete one `submit_turn` / `turn_result` exchange through the browser WebSocket API. |
| Diagnosis room live browser smoke harness | M5 | manual | `make diagnosis-live-browser-smoke`; not run in CI because it requires a real OpenClarion backend/worker stack, a valid bearer token, and a sandbox-capable worker. It can use an existing `OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID` or create one from `OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID` through `POST /api/v1/diagnosis/rooms`, then runs the production Next.js route with `web/playwright.live.config.ts` and requires one connect -> `state` -> `submit_turn` -> `turn_result` browser round trip before writing a JSON proof with request metadata and structured browser observations. |
| Diagnosis room live proof validator | M5 | landed | `make diagnosis-live-smoke-output-test`; validates `scripts/diagnosis_live_smoke_output`, the offline checker that rejects symlink/non-regular proof files plus duplicate-key, unknown-field, weak, or log-polluted `make diagnosis-live-browser-smoke` JSON proofs before they can support M5 live acceptance claims. The retained proof must use canonical scalar fields for `checked_at` as UTC RFC3339, evidence snapshot IDs, completed-turn text, created-room run IDs, and a bounded single-line `evidence` summary that mentions the `turn_result` round trip; keep session, workflow, and run IDs single-line, whitespace-free, and bounded; bind the exercised request mode, session id, evidence snapshot id, message length, and lowercase submitted-message SHA-256 digest to the top-level proof metadata and browser observation without retaining message plaintext; and include browser-observed state load, turn-result observation, submitted-message visibility, connected status after the turn, assistant-turn count increment, transcript message pair increment, `Turn N completed` log consistency with the assistant turn count, and transcript count consistency with the user+assistant pair model. |
| OpenAPI -> TS type freshness | M3 | landed | `make openapi-ts-fresh`; regenerates `web/src/lib/api/openapi.ts` from `api/openapi.yaml` via `openapi-typescript` and fails on diff |
| Temporal trace propagation tests | M3 | landed | covered by `go test ./internal/observability/tracing` and runtime wiring tests in `go test ./cmd/openclarion` |
| Agent runtime dependency and hardcoding boundary | M4 | landed | `make forbidden-agent-runtime`; reads `docs/design/ci/agent-runtime-forbidden.tsv`, validates the policy table for scope coverage, duplicate rows, and unpadded patterns, rejects the listed runtime-family dependencies in first-party control-plane manifests, and rejects those names in non-test first-party source under `cmd/`, `internal/`, `scripts/`, and `web/src/`, until [agent-runtime-selection.md](../agent-runtime-selection.md) accepts a sandbox runtime baseline. The policy is configurable governance data, not production code binding. |
| ContainerProvider contract tests | M4 | landed | `go test ./internal/usecases/ports ./internal/providers/container/fake`; covers ADR-0013 fixed workspace paths, duplicate-key-free request/raw-result JSON validation, timeout/output caps, network policy shape, deterministic fake scripts, copy isolation, scripted errors, and context cancellation |
| Container sandbox security spec gate | M4 | landed | `make sandbox-security`; covers the provider-neutral contract plus Docker runtime-spec validation for digest-pinned images, non-root user, readonly rootfs, `no-new-privileges`, unprivileged execution, dropped capabilities, memory/CPU limits, readonly `/workspace/*` input mounts, writable `/workspace/out` output mount, output-size cap, Docker socket mount rejection, and Docker Provider output archive rejection for unexpected, nested, traversal, symlink, and hardlink members before accepting the single top-level regular `output.json` member |
| Agent tool helper contract tests | M4 | landed | `make agent-tool-scripts-test`; covers the read-only Prometheus instant-query helper and static topology lookup helper used by future sandbox runtime images. The metric helper rejects duplicate-key, unknown-field, and trailing-value Prometheus response JSON before emitting output; the topology helper rejects symlink/non-regular topology files plus duplicate-key, unknown-field, trailing-value, oversized, and semantically invalid topology JSON before emitting lookup output. |
| Sandbox baseline audit | M4 | landed | `make sandbox-baseline-audit`; emits a code-level JSON proof for ADR-0013 file paths, network-none batch defaults, M5 turn input mounts, Docker security posture, resource limits, allowlist subset enforcement, and strict request/raw-output JSON validation. It complements manual Docker smokes rather than replacing them. |
| Sandbox quality comparison helper tests | M4 | landed | `make sandbox-quality-compare-test`; covers the offline direct-vs-sandbox SubReport comparator, regular-file/no-symlink checks before reading manifests and report inputs, duplicate JSON object key rejection in manifests and report inputs, unknown-field rejection in manifests, manifest batch mode, required single-line bounded `sample_basis`, single-line bounded case IDs, per-case alert scenario labels, single-line bounded `required_evidence_refs` binding across both reports, canonical `snapshot:<positive-id>` EvidenceSnapshot refs, scenario coverage output, single-line bounded portable unique report paths, production schema reuse, equivalent/improved/regressed recommendations, aggregate summaries, and fail-on-regression behavior. This is a comparison harness, not a real report-quality acceptance result. |
| Sandbox M4 decision gate tests | M4 | landed | `make sandbox-m4-decision-test`; covers the offline proceed/iterate/defer decision helper that combines baseline audit output, manifest-mode quality comparison output, runtime smoke evidence, and human review evidence while rejecting symlink/non-regular evidence files, duplicate JSON object keys, unknown fields, and duplicate baseline audit check names in evidence inputs. Quality comparison evidence must cover `single_alert`, `cascade`, and `alert_storm`; every case must retain single-line bounded IDs and non-empty single-line bounded `required_evidence_refs` with a canonical `snapshot:<positive-id>` EvidenceSnapshot ref; aggregate summary counts must match `cases[].recommendation`; review evidence `sample_basis` must match the quality comparison `sample_basis`; review evidence must cover the same single-line bounded case IDs via `reviewed_cases`; `selected_candidate` and `candidate_evaluations` are generic unpadded whitespace-free evidence-supplied IDs, with the selected candidate required to have a matching `pass` evaluation whose immutable `runtime_candidate` equals the top-level digest-pinned `runtime_candidate` and whose `runtime_smoke_refs` cite every required runtime smoke name; review evidence human-authored text is required where applicable, single-line, unpadded, and bounded to 2048 bytes; runtime smoke names must be exactly the required smoke set, sources must match their canonical `make` targets, and each smoke must carry a distinct bounded normalized relative `evidence_ref` plus lowercase `evidence_sha256`, preventing free-form pass claims or local-machine-specific paths from satisfying the decision gate. If incomplete evidence and explicit failure evidence coexist, the decision remains `defer` but retained reasons include both classes. The manual `make sandbox-m4-decision` target requires real evidence files and is not auto-run in CI. |
| Sandbox M4 evidence packet tests | M4 | landed | `make sandbox-m4-evidence-packet-test`; covers the manual packet assembler and offline retained-packet verifier. Assembly freezes baseline audit, manifest quality comparison, copied quality manifest and raw direct/sandbox SubReport inputs, copied review evidence, copied runtime-smoke artifacts, decision output, and packet summary into one empty evidence directory. The assembler rejects weak artifacts before writing them: baseline output must contain uniquely named required pass checks, quality output must be manifest-mode with sample basis, required scenario coverage, unique single-line bounded case IDs, per-case review markers, per-case single-line bounded required evidence refs, and summary counts derived from case recommendations; copied quality manifest sample basis, case IDs, scenarios, and required refs must match the generated comparison output; copied direct/sandbox SubReports must pass the production SubReport parser and contain every declared required evidence ref; review evidence must contain dated representative-sample/runtime-smoke/human-review fields with generic unpadded whitespace-free `selected_candidate` / `candidate_evaluations`, a digest-pinned `runtime_candidate`, a matching selected-candidate evaluation runtime ref, selected pass-candidate `runtime_smoke_refs` for every required runtime smoke name, reviewer/status/notes metadata, exactly required runtime smoke names with canonical sources, distinct bounded normalized relative runtime-smoke `evidence_ref` values and lowercase `evidence_sha256` digests, case-level single-line bounded `reviewed_cases`, and the same sample basis as the generated quality comparison, duplicate JSON object keys and unknown fields must be absent, human-authored review text must be single-line, unpadded, and bounded to 2048 bytes, runtime-smoke source artifacts must exist under the configured artifact root, be regular files rather than symlinks or other non-regular files, and match their declared SHA-256 digests before being copied into packet-local `runtime-smoke-artifacts/` paths, decision output must use a valid proceed/iterate/defer value with decision-consistent non-blank, non-duplicate reasons, and decision evidence must match the frozen baseline/quality/review inputs plus `--min-cases`, selected-candidate summary, reviewed-case count, and reviewed case IDs. The retained `packet.json` records `out_dir` as `.`, stores generated artifact and helper output paths as packet-local slash-separated paths, and records SHA-256 digests for the frozen baseline, quality, review, decision, quality-input, and runtime-smoke artifacts. Verification mode `make sandbox-m4-evidence-packet-verify PACKET_DIR=...` revalidates an existing packet without rerunning helpers and rejects stale command metadata, digest drift, missing copied inputs, quality manifest/output drift, copied report schema/ref failures, runtime-smoke artifact drift, decision evidence mismatches, unexpected packet files/directories, symlinks, non-regular files, and `PACKET_JSON` aliases that do not match the packet's own `artifacts.packet` reference. |
| Diagnosis room policy tests | M5 | landed | `make diagnosis-room-policy-test`; covers the pure short-conversation policy boundary for configured turn/time/message/context limits, duplicate message IDs, in-flight turn rejection, basic unsafe-instruction denylist matching, context byte accounting, and strict V1 sandbox `output.json` schema validation. |
| Diagnosis room workflow tests | M5 | landed | `make diagnosis-room-workflow-test`; covers the Temporal room control plane plus per-turn sandbox, transcript persistence, lifecycle audit, and close-notification Activity boundaries: room workflow start options/readiness query through `DiagnosisRoomStarter`, `submit-turn` Update with policy Validator, `EnsureDiagnosisRoomSession`, `EnsureDiagnosisChatSession`, `RunDiagnosisTurn`, `PersistDiagnosisTurn`, `CloseDiagnosisChatSession`, `SendDiagnosisRoomCloseNotification`, `ContainerProvider.Run` request construction, schema-valid assistant output acceptance, idempotent user+assistant ChatTurn persistence, idempotent opened/turn-persisted/closed/close-notification `DiagnosisTaskEvent` audit rows, terminal `ChatSession` close metadata, idempotent IMProvider close notification, workflow-state transcript updates after persistence, `state` Query, close/cancel Signals, durable idle/session timers, duplicate `message_id` rejection, concurrent Update rejection while an Activity is in flight, configured max-turn rejection, unsafe-message rejection, and Temporal client start/Update/Query option mapping. |
| Diagnosis auth boundary tests | M5 | landed | `make diagnosis-auth-test`; covers provider-neutral AuthProvider DTOs, OIDC ID-token verification and role-claim mapping, deterministic auth fake scripts, owner/admin RBAC, short-lived single-use WebSocket ticket issuance/consumption, TTL expiry, wrong-session rejection, defensive in-memory store copies, PostgreSQL-backed ticket persistence with hashed tokens plus concurrent single-consume protection, HTTP/WS transport handshake coverage, authenticated `submit_turn` / `query_state` relay, bounded Update timeout error frames, and disconnect behavior that does not cancel submitted Updates. |
| Diagnosis chat persistence tests | M5 | landed | `make diagnosis-chat-persistence-test`; covers ChatSession/ChatTurn domain invariants plus PostgreSQL-backed schema/repository behavior: unique `session_key`, one session per `DiagnosisTask`, append-only turn insert, per-session `message_id` idempotency, per-session sequence uniqueness, ordered transcript listing, and invariant guards. |
| Docker Provider live smoke harness | M4 | landed / manual | `make container-provider-smoke`; runs `ContainerProvider.Run` through a real local Docker daemon, digest-pinned image ref, network-none, non-root user, readonly rootfs, readonly ADR-0013 input mounts, `/workspace/out` as the only writable mount with `fsize` cap, Docker output archive copy, duplicate-key-free JSON-object output validation, and invocation-label leak check. Not run in CI because it requires a local Docker daemon. |
| Docker Provider timeout cleanup smoke | M4 | landed / manual | `make container-provider-timeout-smoke`; runs the same Docker-backed Provider with a short timeout and long-running command, expects `context deadline exceeded`, then fails if any invocation-labelled container remains. Not run in CI because it requires a local Docker daemon. |
| Docker Provider output cap smoke | M4 | landed / manual | `make container-provider-output-cap-smoke`; runs the same Docker-backed Provider with a 64-byte output cap and a command that writes more than the cap, accepts either `fsize`-enforced process failure or Provider output-size rejection, then fails if any invocation-labelled container remains. Not run in CI because it requires a local Docker daemon. |
| Egress allow-deny smoke harness | M4 | landed / manual | `make egress-allowdeny-smoke`; creates Docker sandbox/upstream networks plus an allowlist proxy, proves allowed-via-proxy succeeds, denied-via-proxy returns 403, and direct sandbox bypass fails. Not run in CI because it requires a local Docker daemon and mutable Docker networks. |
| Manual agent runtime adapter smoke harness | M4 | landed / manual | `make agent-runtime-smoke`; runs a digest-pinned candidate image through network-none, non-root, readonly rootfs, `no-new-privileges`, dropped capabilities, memory/CPU/PID limits, readonly ADR-0013 input mounts, writable `/workspace/out` output mount with `fsize` cap, timeout cleanup, regular-file/no-symlink output-artifact validation, bounded reads, and duplicate-key-free non-empty JSON-object output validation |
| Custom thin runner candidate smoke | M4 | landed / manual | `make custom-thin-runner-smoke`; builds the local scratch-based custom runner plus packaged metric/topology helper binaries, pushes it to an ephemeral localhost registry to obtain a real `repo@sha256` reference, proves the packaged topology helper with `--entrypoint`, then runs both `make agent-runtime-smoke` and `make container-provider-smoke`. The runner rejects symlink/non-regular ADR-0013 JSON input paths plus duplicate-key or trailing-value ADR-0013 input JSON before hashing mounted inputs. This is a contract/lifecycle proof, not a report-quality baseline. |
| Agent runtime adapter candidate result | M4 | partial | custom thin runner contract proof and offline quality comparison harness landed; OpenClaw/Hermes framework candidates and real report-quality comparison remain pending before any runtime baseline is accepted |
| Container sandbox runtime execution gate | M4 | landed / manual | `make container-provider-smoke` proves create/start/wait/copy/remove plus duplicate-key-free JSON output on a short-lived sandbox; `make container-provider-timeout-smoke` proves timeout stop/remove cleanup; `make container-provider-output-cap-smoke` proves output cap enforcement against a real local Docker daemon. |
| WebSocket auth handshake and relay test | M5 | landed | covered by `make diagnosis-auth-test`; verifies `POST /api/v1/diagnosis/ws-ticket`, ticket-required `GET /ws/diagnosis`, non-upgrade rejection before ticket consumption, same-origin rejection before ticket consumption, authenticated connection handoff, `submit_turn` to Temporal Update, `query_state` to Temporal Query, timeout response, and disconnect behavior |
| Bounded-turn enforcement test | M5 | landed | covered by `make diagnosis-room-workflow-test`; guards against client-side bypass at the workflow Update Validator |
| Audit completeness test | M5 | landed | covered by `make diagnosis-room-workflow-test`; verifies opened, turn-persisted, closed, and close-notification lifecycle events are idempotently logged through `DiagnosisTaskEvent` |

## Current Private-Incubation Gate

The canonical entry point is `make pr` (and `make ci`), defined in the
repository root `Makefile`. GitHub Actions calls the same `make` targets.

```bash
make pr            # full PR validation bundle
make docs-hygiene  # documentation language and terminology gate
make forbidden     # forbidden-method bundle (latest / v2 / sqlite; import rules live in make go-lint)
make adr-check     # ADR index consistency
make links-check   # markdown link, anchor, and docs reachability validation
make external-links-check # external link inventory; set OPENCLARION_EXTERNAL_LINKS_LIVE=1 for live liveness
make markdownlint  # markdown structure/style lint
make doc-claims-check # shipped CURRENT_STATE.md path claims
make gate-hardening-check # CI gate maturity checklist coverage
make allowlist-discipline # allowlist owner / expiry / removal metadata
make workflow-change-guard # PR-only workflow-file change isolation
make pr-budget-test # make pr wall-clock budget wrapper tests
PR_TITLE='feat: concise title' make pr-title-check # PR title policy
PR_BODY="$(gh pr view --json body --jq .body)" make pr-description-check # PR body policy
PR_BODY="$(gh pr view --json body --jq .body)" IMPACT_REFERENCE_BASE_REF=main IMPACT_REFERENCE_HEAD_SHA=HEAD make pr-impact-reference-check # PR issue/ADR reference policy
make generated-headers # generated file header validation
make generate-fresh # make generate freshness and idempotence
make go-coverage # handwritten Go package coverage floor
make testcontainers-contract # integration-test DB and host-network contract
make openapi-breaking # OpenAPI breaking-change diff (W4 soft-fail until 2026-06-10)
make openapi-fingerprint # critical OpenAPI node lock
make go-licenses-check # Go dependency license compliance
make osv-scan # npm package-lock vulnerability scan
make sandbox-security # M4 sandbox contract and Docker security-spec tests
make agent-tool-scripts-test # M4 sandbox metric/topology helper contract tests
make sandbox-baseline-audit # M4/M5 code-level sandbox baseline audit
make sandbox-quality-compare-test # M4 offline direct-vs-sandbox SubReport comparison tests
make sandbox-m4-decision-test # M4 offline proceed/iterate/defer decision tests
make sandbox-m4-decision # manual M4 decision with evidence file inputs
make sandbox-m4-evidence-packet-test # M4 evidence packet assembly tests
make sandbox-m4-evidence-packet # manual M4 evidence packet assembly
make sandbox-m4-evidence-packet-verify # manual M4 retained packet verification
make report-live-smoke-output-test # M2 live report smoke proof validator tests
make diagnosis-room-policy-test # M5 pure short-conversation policy tests
make diagnosis-room-workflow-test # M5 Temporal diagnosis room workflow/client and lifecycle activity tests
make diagnosis-auth-test # M5 AuthProvider/OIDC/RBAC/WS ticket boundary + persistence + transport relay tests
make diagnosis-chat-persistence-test # M5 ChatSession/ChatTurn persistence tests
make diagnosis-live-smoke-output-test # M5 live browser smoke proof validator tests
make container-provider-smoke # manual M4 Docker Provider.Run smoke; requires Docker
make container-provider-timeout-smoke # manual M4 Docker Provider timeout cleanup smoke; requires Docker
make container-provider-output-cap-smoke # manual M4 Docker Provider output cap smoke; requires Docker
make egress-allowdeny-smoke # manual M4 egress proxy allow/deny smoke; requires Docker
make agent-runtime-smoke # manual M4 candidate image smoke; requires digest-pinned image
make custom-thin-runner-smoke # manual M4 local custom runner candidate smoke; requires Docker
make report-live-smoke # manual M2 live external smoke; requires real services
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
  `@v4` are forbidden. The version comment is enforced by
  `workflow-parity`. See `docs/design/DEPENDENCIES.md`.
- PR-triggered workflows declare top-level `concurrency:` with
  `cancel-in-progress: true`.
- `pull_request` workflows may not reference `${{ secrets.* }}`. If a future
  workflow genuinely needs PR-time secrets, it must use `pull_request_target`
  and include a `# pull-request-target-review-policy: <review process>` marker
  in the workflow file so the reviewer policy is visible in code review.
- Workflows declare top-level `defaults.run.shell: bash` so `run:`
  steps do not inherit runner defaults.
- Workflow files use `ci.yml` for the primary gate bundle or
  `<gate>.yml` for narrowly scoped gate workflows, and every committed
  workflow file is listed in the Workflow File Registry below.
- Top-level workflow `name:` values are required and unique across
  `.github/workflows/` so branch-protection and failure names stay
  unambiguous.
- Every job uses a fixed Ubuntu runner label (`ubuntu-24.04` or
  `ubuntu-22.04` during M0-M2).
- Every job declares an explicit `permissions:` block (start from
  `contents: read`) and a `timeout-minutes:` value capped at 15
  minutes during M0-M2.
- Workflow-level and job-level `permissions:` entries may not exceed
  `contents: read` unless the broader entry carries an inline
  `# parity-allow: <reason>` justification. This keeps write scopes
  reviewable instead of silently broadening the default token.

### Workflow File Registry

| Workflow file | Purpose |
|---|---|
| `.github/workflows/ci.yml` | Primary PR/push CI gate bundle; every job calls repository-owned `make` targets |

### Lock File Discipline

- Lock files are generated from committed source-of-truth files, not
  hand-authored from assumed future paths.
- A PR that changes a locked node must update the corresponding lock
  in the same change and let the gate prove the new fingerprint.
- A lock may not contain stale entries: if a locked node disappears,
  the check fails until the source and lock are deliberately reconciled.
- Lock files must be strict JSON artifacts: duplicate object keys and trailing
  JSON values are rejected before comparisons run.
- The current OpenAPI lock covers the committed critical domain API nodes,
  including alert/evidence reads, dashboard, reports, replay trigger, and
  diagnosis-room creation/ticket paths.

## Future Imports from Shepherd Reference

The shepherd-platform reference project has many ready CI assets. Importing
everything now would create empty gates that depend on code OpenClarion has
not yet committed. The list below records what to import **when the
corresponding code lands**, with a one-line description so the import is
deliberate rather than copy-paste. Do not pre-stage these assets; pull each
one from shepherd at the milestone that activates it.

| Asset (shepherd path) | Purpose | Activate at | Notes |
|-----------------------|---------|-------------|-------|
| `scripts/run_pr_parallel.sh` | parallel `make pr` job runner for local development speed | M2+ (when sequential `make pr` exceeds ~10 min) | premature on M0/M1 |
| `scripts/check_public_hygiene.sh` | private-marker scanner for public-incubation transition | private-to-public transition | needed only if/when OpenClarion exits private incubation |
| `release-please-config.json` + `.github/workflows/release-please.yml` + `.github/workflows/release.yml` + `docs/RELEASE.md` | conventional-commit-driven release automation | M3-M4 (first public preview) | conventional commit discipline already enforced in `ai-code/.agent/skills/github-workflow/SKILL.md` so adoption is non-disruptive |
| `docs/design/ci/scripts/check_*.go` static analyzers | future provider-wiring / sqlc / SSA boundary checks not covered by `tools/openclarion-linter` | M2-M4 (per analyzer's target) | port one analyzer at a time, only when the analyzed code exists in OpenClarion |

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

### DCO gate (`make dco-check`)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Forbid GitHub `noreply` addresses in author email (project policy decision) | when project decides whether to allow web-edit commits | regex `@users\.noreply\.github\.com$` reject |
| Require GPG / SSH commit signature verification (`git verify-commit`) on PR commits | when contributor base is large enough that DCO trailer alone is insufficient | call `git verify-commit` on each SHA; allowlist maintainers initially |

### Forbidden-imports / architectural boundaries

| Tightening | Trigger | Implementation hint |
|---|---|---|

### Dependency pinning (`make forbidden-latest` family)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Reject indirect dependencies older than N months without an `indirect` allow-list | M2 | query Go module proxy or OSV metadata for module release dates; cross-reference an expiry-based allow-list |

### ADR governance gate (`make adr-check`)

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Enforce 48-hour governance window via PR labels rather than honor system | M2 (when external contributors arrive) | bot adds `adr-window-start` timestamp label; merge gate checks elapsed time |

### Markdown / docs hygiene (`make links-check`, `make docs-hygiene`, `make markdownlint`)

| Tightening | Trigger | Implementation hint |
|---|---|---|

### Code generation freshness

| Tightening | Trigger | Implementation hint |
|---|---|---|

### Supply-chain & secrets

| Tightening | Trigger | Implementation hint |
|---|---|---|
| SBOM generation (CycloneDX) attached to every release artifact | M3-M4 | `cyclonedx-gomod` for Go, `@cyclonedx/cdxgen` for npm; integrate into release-please pipeline |
| Container image vulnerability scan (Trivy / grype) on push | M4 | runs against the digest pinned in the build pipeline |
| Sigstore / cosign artifact signing | M4 | optional but recommended once we ship containers |

### Test discipline

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Benchmark regression gate (>10% slowdown blocks PR) | M3 | `benchstat` comparison; soft-failure first, hard-failure once stable |

### Performance & cost budgets

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Workflow run-time alerting (median + p95 dashboard) | M3 | scheduled report job; not PR-blocking |
| CI artifact size cap (per artifact, total per PR) | M3 | post-job inventory; surfaces accidental binary commits |
| Cache hit rate floor for module / npm caches | M3 | observability only; informs cache key tuning |

### PR / commit governance

| Tightening | Trigger | Implementation hint |
|---|---|---|
| PR file count cap (default: 50; opt-out with maintainer label) | M2 | API call from a dedicated workflow |
| Squash-merge enforced as the only allowed merge strategy | M1 | repo setting + a guard workflow that warns on merge-commit shape |
| Branch protection: require all M0 jobs as required checks | M1 | configured in repo settings, recorded here as the policy source |
| Negative-test fixture per gate (`tests/ci/negative/<gate>.sh`) committed alongside the gate | M2 | repository convention; the existing `parity_neg_v2.sh` is the prototype |

### Documentation of these rules

| Tightening | Trigger | Implementation hint |
|---|---|---|
| Move each activated row out of this section and into the Schedule table | continuous | each PR that lands a hardening item also deletes its row here |
| Keep `make pr` as the primary reproducible local CI command, with explicit documentation for PR-context-only exceptions | always | DCO is the current exception because its authoritative range exists only on `pull_request`; record any new deviation as a deliberate ADR |
