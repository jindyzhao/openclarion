# Dependency Policy

## Baseline

| Area | Dependency | Validated |
|------|------------|----------|
| Go | Go 1.25+ (toolchain pinned in `go.mod` at M0) | 2026-05-19 |
| HTTP | std `net/http` (Go 1.22+ enhanced routing) | 2026-05-19 |
| API | OpenAPI 3.1, `oapi-codegen-exp` V3 (pinned in `go.mod` at M0) | 2026-05-22 |
| Database | PostgreSQL 18, Ent `v0.14.6` (`go.mod` direct require + `tool` directive at M1-PR1), Atlas CLI `arigaio/atlas:1.2.0` (Docker image pin) | 2026-05-22 |
| Workflow | Temporal Go SDK `v1.44.0` (pinned at M1-PR3 via first-import rule, per ADR-0012 amendment) | 2026-05-25 |
| Frontend | Node.js 24.x LTS, React 19, Next.js 16 | 2026-05-19 |
| Observability | OpenTelemetry Go (pinned at M3 first-import), Prometheus client (pinned at M3 first-import) | 2026-05-22 |
| Future vector | pgvector 0.7+ (not MVP) | 2026-05-19 |

> **First-import pin rule**: a Go dependency is added to `go.mod` only when
> production code first imports it. The version is then pinned to a concrete
> `module@version` (no `latest`). The CI gate `forbidden-latest` rejects the
> literal string `latest` in `go.mod` and `package.json`.

## Container Image Pinning Policy

Different environments have different pinning requirements:

| Environment | Pin granularity | Rationale |
|-------------|-----------------|-----------|
| Local `docker-compose.yml` (dev) | tag with major+minor (e.g. `postgres:18-alpine`); `latest` forbidden | reproducibility within a release line; upgrade burden stays manageable for individual contributors |
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
- The literal string `latest` is forbidden in `go.mod` and `package.json`
  (enforced by `make forbidden-latest`).

## Risk Assessment

| Dependency | Risk | Mitigation |
|------------|------|------------|
| `oapi-codegen-exp` V3 | experimental, pre-v1, API may change | pinned to `v0.1.0` in `go.mod`; CI validates generated output compiles; fallback path: downgrade to v2 with `openapi.compat.yaml` bridge |
| Temporal operational complexity | adds infrastructure dependency | use PostgreSQL persistence (shared cluster, separate database); defer Cassandra/Elasticsearch until scale demands |

## Downgrade Paths

- **oapi-codegen-exp to v2**: reintroduce `api/openapi.compat.yaml` (3.0 bridge)
  and switch generator to `oapi-codegen/v2`. Requires a superseding ADR for
  ADR-0007.
- **Node.js 24 to 22**: Node.js 22 remains in maintenance until April 2027.
  Downgrade is safe if a dependency requires it.
- **Agent sandbox runtime**: ContainerProvider interface accepts any
  OCI-compliant runtime. The sandbox interior (Python, Go, or any agent
  framework) is swappable without control-plane changes.
