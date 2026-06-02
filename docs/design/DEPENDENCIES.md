# Dependency Policy

## Baseline

| Area | Dependency | Validated |
|------|------------|----------|
| Go | Go 1.25+ (toolchain pinned in `go.mod` at M0) | 2026-05-19 |
| Custom analyzer tooling | `golangci-lint v2.12.2` plus `tools/openclarion-linter` `golang.org/x/tools v0.44.0`; the versions must match exactly because `make go-lint` builds a custom golangci-lint module plugin | 2026-05-30 |
| CI configuration parsing | `go.yaml.in/yaml/v3 v3.0.4` (direct require since repository-owned Go checkers parse workflow, golangci-lint, OpenAPI, and Dependabot YAML with known-field validation where supported) | 2026-05-30 |
| Go module manifest parsing | `golang.org/x/mod v0.35.0` (direct require since `scripts/agent_runtime_policy_check` parses `go.mod` require/replace entries structurally for the `make forbidden-agent-runtime` control-plane runtime boundary) | 2026-05-30 |
| HTTP | std `net/http` (Go 1.22+ enhanced routing) | 2026-05-19 |
| WebSocket transport | `github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674` (direct require since `internal/transport/http` upgrades authenticated diagnosis-room connections with explicit same-origin checks and `httptest`/Dialer coverage) | 2026-05-28 |
| API | OpenAPI 3.1, `oapi-codegen-exp` V3 (pinned in `go.mod` at M0) | 2026-05-22 |
| API diff | `oasdiff v1.11.7` via pinned `go run github.com/oasdiff/oasdiff@v1.11.7` in `make openapi-breaking` | 2026-05-27 |
| Database | PostgreSQL 18, Ent `v0.14.6` (`go.mod` direct require + `tool` directive at M1-PR1), Atlas CLI `arigaio/atlas:1.2.0` (Docker image pin) | 2026-05-22 |
| Workflow | Temporal Go SDK `go.temporal.io/sdk v1.44.0` pinned via first-import rule at M1-PR3 (`DiagnosisWorkflow` shell shipped per ADR-0012 amendment) | 2026-05-25 |
| Frontend / Node tooling | Node.js 24.x LTS in CI, Next.js `16.2.6`, React / React DOM `19.2.6`, TypeScript `5.9.3`, ESLint `9.39.4` + `eslint-config-next 16.2.6`, Vitest `4.1.7`, Playwright `1.60.0`, `@types/node 24.12.4`, Knip `6.14.2`, OpenAPI TypeScript `7.13.0`, Markdownlint CLI2 `0.22.1`; `postcss` is overridden to `8.5.15` to stay above the advisory floor while Next's dependency graph catches up | 2026-05-30 |
| Observability | OpenTelemetry Go `go.opentelemetry.io/otel v1.44.0`, `go.opentelemetry.io/otel/sdk v1.44.0`, OTLP HTTP trace exporter `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0`, HTTP server/client instrumentation `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0`, and Temporal OTel interceptor `go.temporal.io/sdk/contrib/opentelemetry v0.7.0` (direct requires since `internal/observability/tracing` initializes W3C propagation, no-op/OTLP tracer providers, resource service attributes, generated API HTTP span middleware, outbound HTTP transport instrumentation, Temporal workflow/activity tracing, and an OTLP HTTP collector smoke; exporter pin is above the `GO-2026-4985` fixed-in floor reported by `govulncheck`) | 2026-06-03 |
| Metrics ingest + exposition | Prometheus client `github.com/prometheus/client_golang v1.23.2` + `github.com/prometheus/common v0.67.5` (direct require since `internal/providers/metrics/prometheus/client.go` imports both `common/config` for the Bearer-auth round-tripper and `common/model` for `LabelSet`; M3 `/metrics` exposition also uses `prometheus`, `collectors`, and `promhttp` from the same pinned module) | 2026-05-28 |
| LLM output validation | `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2` (direct require since `internal/usecases/llmoutput` validates provider JSON against report schemas before persistence; default draft 2020-12) | 2026-05-28 |
| Docker Engine sandbox provider | Docker Go SDK modules `github.com/moby/moby/api v1.54.2` and `github.com/moby/moby/client v0.4.1` (direct requires since `internal/providers/container/docker/provider.go` imports Engine API types and the official client for create/start/wait/stop/kill/remove/copy lifecycle calls; unit tests use a fake EngineClient so cleanup, timeout, and output-copy behavior are verified without requiring a local daemon) | 2026-06-03 |
| Authentication | `github.com/coreos/go-oidc/v3 v3.18.0` (direct require since `internal/providers/auth/oidc` verifies signed OIDC ID tokens through issuer discovery/JWKS, client ID audience checks, expiry/signature validation, and role-claim extraction for M5 AuthProvider) | 2026-05-28 |
| Future vector | pgvector 0.7+ (not MVP) | 2026-05-19 |

> **First-import pin rule**: a Go dependency is added to `go.mod` only when
> production code first imports it. The version is then pinned to a concrete
> `module@version` (no `latest`). Critical first-import modules that define
> platform boundaries (Ent, Temporal SDK, and OTel) must remain direct
> root-module `require` pins and may not be redirected with `replace`.
> Go tools declared through the Go 1.24+ `tool` directive must also resolve to
> a concrete `require` version pin; the `tool` entry names the executable
> package, while the `require` entry pins the module that provides it.
> Any other committed Go `replace` directive is a temporary fork/local-path
> override and must be explicitly allowlisted in this file with
> `replace-allow: <module> => <target>; owner: <owner>; expires:
> YYYY-MM-DD; reason: <reason>` before merge.
> First-party npm manifests also use exact dependency versions. The CI gate
> `forbidden-latest` rejects the literal string `latest` in `go.mod` and
> `package.json`, rejects `^` / `~` ranges in `package.json`, enforces the
> critical direct Go module pins, verifies Go `tool` directive backing pins,
> rejects undocumented Go `replace` directives, and rejects external
> Dockerfile base images that are not pinned to an immutable `@sha256:` digest.
> Frontend Node type definitions must track the CI Node.js runtime major:
> `@types/node` major bumps are coordinated with the workflow Node.js baseline
> instead of being accepted as standalone dependency updates, and
> `forbidden-latest` rejects mismatched `@types/node` / `setup-node` majors.

> **Custom analyzer lockstep rule**: `tools/openclarion-linter` must keep
> `golang.org/x/tools` on the exact version embedded in the pinned
> `GOLANGCI_LINT_VERSION` binary. `scripts/check_lint_version.sh` enforces this
> before `golangci-lint custom` runs. Dependabot therefore suppresses ordinary
> minor/major `golang.org/x/tools` version-update PRs for the linter submodule;
> those updates must land only with a coordinated `GOLANGCI_LINT_VERSION` bump.
> Security updates are still handled by Dependabot security-update PRs and must
> either preserve parity or carry an explicit coordinated tooling update.

> **Frontend toolchain major-update rule**: Dependabot suppresses ordinary
> semver-major version-update PRs for `/web` `typescript` and `eslint`.
> Those upgrades are treated as coordinated toolchain migrations because they
> must be validated together with Next.js, `eslint-config-next`, generated
> OpenAPI TypeScript output, lint, typecheck, build, and browser smoke gates.
> Dependabot security-update PRs remain allowed and must either pass the same
> gates or carry an explicit rollback path.

## License Compliance Policy

Go dependency licenses are checked with pinned `go-licenses v1.6.0` through
`make go-licenses-check`. The gate includes test dependencies and scans both
the root Go module and `tools/openclarion-linter`; first-party OpenClarion
package prefixes are ignored in `go-licenses` so the gate evaluates third-party
dependencies while still traversing dependencies imported from those packages.
The SPDX allowlist line must carry non-empty owner, non-future review date, and
reason metadata.

```text
go-license-allow: Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC,MIT,MPL-2.0; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency SPDX IDs for current runtime, tooling, and test dependency graph
```

GPL, AGPL, LGPL, unknown, or unclassified Go dependency licenses are not
accepted by default. A future exception must be recorded here with a concrete
owner, review date, and reason in the same change that introduces the
dependency.

## Container Image Pinning Policy

Different environments have different pinning requirements:

| Environment | Pin granularity | Rationale |
|-------------|-----------------|-----------|
| Local `docker-compose.yml` (dev) | tag with major+minor (e.g. `postgres:18-alpine`); `latest` forbidden | reproducibility within a release line; upgrade burden stays manageable for individual contributors |
| First-party Dockerfile base images | `@sha256:...` digest for external images; `scratch` and previous build stages allowed | build/runtime images are executable supply-chain inputs once M4 sandbox images exist; internal stage reuse and scratch images do not pull a mutable external base |
| Build-time tooling images (e.g. `arigaio/atlas:1.2.0`) | tag with major+minor+patch; `latest` and rolling tags forbidden | the tool runs against schema/migration source-of-truth files, so deterministic CLI behaviour matters more than tracking a release line |
| GitHub Actions (`uses:`) | full 40-char commit SHA | external code execution surface; pinning prevents supply-chain drift; Dependabot handles updates |
| M4 sandbox runtime images | `@sha256:...` digest | non-root, network-restricted security boundary; immutability enforced by separate `sandbox-security` CI gate (introduced at M4) |
| Production deployment manifests | `@sha256:...` digest | same rationale as M4 sandbox; verified at release time |

> Promoting `docker-compose.yml` to digest pinning would require introducing
> Renovate or Docker Scout for automated digest refresh; otherwise digest
> pinning becomes a vector for stale base-image security patches. This is
> intentionally deferred until M4 sandbox needs it.

## Atlas CLI Integration Policy

Atlas ships as a self-contained binary, not a `go install`-able module.
M1-PR1 integrates Atlas via the pinned Docker image rather than a
GitHub-Actions-only path so that local and CI invocations execute the
exact same binary.

* **Pinned image**: `arigaio/atlas:1.2.0` (single source of truth: the
  `ATLAS_IMAGE` variable in the root `Makefile`, propagated to wrapper
  scripts via explicit per-target `ATLAS_IMAGE="$(ATLAS_IMAGE)" bash ...`
  recipes). The default in `scripts/lib_atlas.sh` is a fallback ONLY
  for direct script invocation; the canonical entry point is `make`.
  Upgrading Atlas requires updating `Makefile` AND the row in this
  file in the same PR.
* **Invocation pattern**: the host wrapper (`scripts/lib_atlas.sh`)
  launches an ephemeral `postgres:18-alpine` on a per-invocation
  dedicated Docker network and runs Atlas in the pinned image on the
  same network. Atlas talks to the dev DB via a plain
  `postgres://postgres:postgres@<container>:5432/dev?...` URL resolved
  through the network's embedded DNS. The host Docker socket is **NOT**
  mounted; the `docker://...` dev-url form is intentionally NOT used
  (the Atlas image does not ship a Docker CLI). Because the Atlas image
  also does not ship Go, the host Go toolchain (`$(go env GOROOT)`) is
  mounted read-only at `/usr/local/go`; Atlas runs as
  `--user $(id -u):$(id -g)` so generated migration files are owned by
  the invoking user, not root. See `scripts/lib_atlas.sh` and
  `docs/design/database/migrations.md` for the full contract.
* **Acceptance smoke (PR1 first action)**: `make atlas-smoke` proves that
  the chosen image variant can resolve `ent://internal/persistence/ent/schema`
  and reach the dev database. Running this gate at the start of M1-PR1
  is the contract that protects us from discovering an Atlas-image
  capability gap after schema work has already shipped.
* **Fallback if smoke fails**: switch to the
  `ariga/setup-atlas` GitHub Action (must be pinned to a 40-char SHA per
  the workflow-parity gate) and document the alternative path in
  `docs/design/database/migrations.md`. We do NOT work around an image
  failure by re-implementing Atlas semantics in the Makefile.

## Rules

- Core runtime dependencies must be justified by an ADR or design doc.
- Database, workflow, API, and frontend code generation versions are pinned.
- New infrastructure dependencies require an ADR.
- Redis, MongoDB, and external vector databases are not part of the MVP runtime.
- Security updates may bypass normal release batching but must include validation.
- No third-party HTTP framework (Gin, Echo, Fiber) unless a future ADR accepts one.
- The literal string `latest` is forbidden in `go.mod` and `package.json`.
  First-party `package.json` dependency values also cannot use `^` or `~`
  ranges, critical first-import Go modules must remain direct concrete pins
  without `replace`, Go `tool` directive paths must resolve to concrete
  `require` version pins, other committed Go `replace` directives must be
  documented with a `replace-allow: <module> => <target>; owner: <owner>;
  expires: YYYY-MM-DD; reason: <reason>` marker in this file, and committed
  Dockerfiles cannot pull external base images without an immutable
  `@sha256:` digest. These rules are enforced by `make forbidden-latest`.

## Risk Assessment

| Dependency | Risk | Mitigation |
|------------|------|------------|
| `oapi-codegen-exp` V3 | experimental, pre-v1, API may change | pinned to `v0.1.0` in `go.mod`; CI validates generated output compiles; fallback path: downgrade to v2 with `openapi.compat.yaml` bridge |
| `oasdiff` CLI | breaking-change classification can evolve between releases | pinned to `v1.11.7`; gate runs through Makefile and has a documented W4 soft-fail sunset before hard enforcement |
| Temporal operational complexity | adds infrastructure dependency | use PostgreSQL persistence (shared cluster, separate database); defer Cassandra/Elasticsearch until scale demands |

## Downgrade Paths

- **oapi-codegen-exp to v2**: reintroduce `api/openapi.compat.yaml` (3.0 bridge)
  and switch generator to `oapi-codegen/v2`. Requires a superseding ADR for
  ADR-0007.
- **Node.js 24 to 22**: Node.js 22 remains in maintenance until April 2027.
  Downgrade is safe if a dependency requires it.
- **Agent sandbox runtime**: ContainerProvider interface accepts any
  OCI-compliant runtime. The sandbox interior (Python, Go, or any agent
  framework) is swappable without control-plane changes. Runtime-family
  dependencies listed in
  [agent-runtime-forbidden.tsv](ci/agent-runtime-forbidden.tsv), or similar
  framework dependencies, must remain inside candidate sandbox images until
  [agent-runtime-selection.md](agent-runtime-selection.md) accepts a runtime
  baseline; first-party dependency manifests plus non-test source under `cmd/`,
  `internal/`, `scripts/`, and `web/src/` are guarded by the config-driven
  `make forbidden-agent-runtime` policy.
