# Database Migrations

Atlas migrations are the canonical database migration artifacts. The
canonical schema definition is `internal/persistence/ent/schema/`; Atlas
diffs the live ent schema against the committed migration history under
`internal/persistence/migrations/` to produce the next migration.

> **Status (2026-05-22, M1-PR1):** the Atlas wrapper has landed. The
> original draft (Atlas-container with `--dev-url docker://...` and a
> mounted host Docker socket) was empirically rejected:
> `arigaio/atlas:1.2.0` and the alpine / community variants do not
> bundle a Docker CLI, and reading `ent://...` from inside the Atlas
> container also requires a Go runtime that the image does not ship.
> The redesigned wrapper (host script launches a per-invocation
> `pgvector/pgvector:0.8.2-pg18-trixie` on a dedicated Docker network;
> the wrapper enables the `vector` extension in the target database; Atlas runs in
> `arigaio/atlas:1.2.0` with the host Go toolchain mounted read-only at
> `/usr/local/go`, runs as `$(id -u):$(id -g)`, talks to the dev DB via
> plain `postgres://`) is committed in `scripts/lib_atlas.sh` and three
> thin entry scripts (`check_atlas_smoke.sh`, `check_atlas_drift.sh`,
> `atlas_migrate_diff.sh`). `make atlas-drift` is wired into CI (no-op
> until the first migration lands); `make atlas-smoke` is the M1-PR1
> manual one-shot acceptance gate and must pass on the host before the
> remaining four Ent schemas and the first migration land.

## Source-of-truth layout

```
internal/persistence/
├── ent/
│   ├── generate.go              # `go:generate` directive (do not edit by hand)
│   ├── schema/                  # ent schema files — THE ONLY hand-edited source
│   └── ...                      # generated client + entities (committed; ent-fresh enforces no diff)
└── migrations/                  # Atlas-generated SQL files + atlas.sum (committed)
```

## Toolchain

| Tool | Version | Distribution | Purpose |
|------|---------|--------------|---------|
| Ent  | `v0.14.6` | `go.mod` (`require` + `tool` directives) | schema-as-code definition + client generation |
| Atlas CLI | `arigaio/atlas:1.2.0` | pinned Docker image (see `Makefile` `ATLAS_IMAGE`) | migration planning, drift detection |
| Postgres dev DB | `pgvector/pgvector:0.8.2-pg18-trixie` (see `DEV_PG_IMAGE` in `scripts/lib_atlas.sh`) | launched by the host script per invocation on a dedicated Docker network | ephemeral schema-diff sandbox with the `vector` extension enabled |

`go.mod` carries the Ent runtime and the `entgo.io/ent/cmd/ent` codegen
tool; Atlas is NOT in `go.mod` (the indirect `ariga.io/atlas` entry that
appears there is the Atlas Go SDK pulled in by ent's loader, NOT the
CLI). The Atlas CLI is used only via the pinned Docker image.

The report-retrieval migration executes `CREATE EXTENSION IF NOT EXISTS
vector`. Production PostgreSQL must make pgvector available and the migration
role must be allowed to enable it before application rollout. Local Compose and
the Atlas wrapper satisfy this prerequisite with the pinned pgvector image; a
managed service must satisfy it through its own extension-management controls.

## Standard invocation pattern

Atlas is invoked by `scripts/lib_atlas.sh::atlas::run`, which is called
from three thin entry scripts:

* `scripts/check_atlas_smoke.sh` -- manual one-shot acceptance gate
  (`make atlas-smoke`).
* `scripts/check_atlas_drift.sh` -- CI drift gate (`make atlas-drift`).
* `scripts/atlas_migrate_diff.sh` -- manual migration generator
  (`make atlas-migrate-diff NAME=<name>`).

Each entry script first calls `atlas::start_dev_pg` to launch a
per-invocation `pgvector/pgvector:0.8.2-pg18-trixie` on a dedicated Docker network
(unique container / network names so concurrent jobs cannot collide),
waits for the target `dev` database with `SELECT 1`, enables the `vector`
extension, then calls `atlas::run` and `atlas::stop_dev_pg` (registered in
`trap`).

`atlas::run` invokes `arigaio/atlas:1.2.0` with the following
non-negotiable flags:

* `--network <per-run-net>` -- same network as the dev Postgres; no
  `--network host`, no published ports, safe for CI matrix concurrency.
* `--user "$(id -u):$(id -g)"` -- generated migration files are owned
  by the invoking user, not root.
* `-v "$PWD:/workspace" -w /workspace` -- repo mounted at the same
  path as on the host so `ent://internal/...` paths resolve.
* `-v "$GOROOT_HOST:/usr/local/go:ro"` -- host Go toolchain mounted
  read-only; the Atlas image does not ship a Go runtime.
* `-e PATH=/usr/local/go/bin:/usr/bin:/bin -e GOROOT=/usr/local/go`
  -- the Ent loader Atlas spawns is `go run` against the project's
  own ent code.
* `-e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomodcache`
  -- the non-root user has no writable home in the distroless image.

The dev URL passed to Atlas is
`postgres://postgres:postgres@<per-run-pg-name>:5432/dev?search_path=public&sslmode=disable`,
resolved through the dedicated network's embedded DNS. The
`docker://...` dev-url form is intentionally NOT used: the Atlas image
does not bundle a Docker CLI, so Atlas cannot spawn the dev DB itself.

## Make targets

| Target | When to run | What it does |
|--------|-------------|--------------|
| `make ent-generate` | after editing any file under `ent/schema/` | regenerates the ent client + entity code |
| `make ent-fresh` | CI gate (and manual sanity check) | snapshots `internal/persistence/ent/`, runs `ent-generate`, and fails only if generation changes that snapshot. This mirrors `openapi-fresh` semantics and allows an in-progress branch to pass after schema and generated code are updated together. |
| `make atlas-smoke` | once at the start of M1-PR1 (manual) | proves the pinned Atlas image can read `ent://...` and spawn the dev Postgres; throwaway output |
| `make atlas-migrate-diff NAME=<name>` | after schema changes (manual) | writes a new migration to `internal/persistence/migrations/` |
| `make atlas-drift` | CI gate | copies migrations to a temp dir, runs `atlas migrate diff drift_check`, fails if Atlas wants to write a new migration |

## Standard workflow for a schema change

1. Edit a file under `internal/persistence/ent/schema/`.
2. `make ent-generate` (or `make ent-fresh` to also assert no leftover diff).
3. `make atlas-migrate-diff NAME=<descriptive_change_name>`.
4. Inspect the new file under `internal/persistence/migrations/`. Manual
   edits are allowed for ordering, comments, and data-migration glue;
   destructive changes require a migration note and rollback plan
   (see "Rules" below).
5. `make atlas-drift` (sanity check that nothing else drifted).
6. `git add internal/persistence/ent/ internal/persistence/migrations/` and commit.

## Drift gate (`make atlas-drift`) details

The drift gate is the per-PR CI check:

* It copies `internal/persistence/migrations/` into `.atlas-drift-tmp/`
  (gitignored), so the canonical migration directory is never mutated
  by the gate itself.
* It runs `atlas migrate diff drift_check` against the temp dir, with
  `--to ent://internal/persistence/ent/schema` and the per-invocation
  `postgres://...@<dev-pg>:5432/dev?search_path=public&sslmode=disable`
  dev-url described above (see `scripts/check_atlas_drift.sh` for the
  exact invocation; `scripts/lib_atlas.sh` for the wrapper).
* If Atlas writes any new file into the temp dir, the schema and the
  migrations are out of sync. The gate prints the new files and fails.
* The gate is a no-op until the first migration lands; that keeps the
  M1-PR1 wiring permanent without forcing the smoke step to ship at the
  exact same moment as the very first generated migration.

## Smoke gate (`make atlas-smoke`) protocol

The smoke gate is the M1-PR1 acceptance contract: it is run **once**,
manually, before any migration is committed. It proves that the chosen
Atlas image plus the wrapper plus the host toolchain can resolve the
ent schema URL and reach a dev Postgres.

The wrapper described above is the result of the empirical findings
recorded on 2026-05-22; the table below is kept for traceability.

| Symptom (observed in the first wrapper draft) | Root cause | How the current wrapper handles it |
|--------------------|------------|-------------------------------------|
| `exec: "docker": executable file not found in $PATH` when using `--dev-url docker://...` | `arigaio/atlas:1.2.0` (and the alpine / community / community-alpine variants) do not bundle a Docker CLI | Host script (`atlas::start_dev_pg`) launches the dev Postgres before Atlas runs; Atlas uses a plain `postgres://...` dev-url and the host Docker socket is NOT mounted. |
| `exec: "go": executable file not found in $PATH` when using `--to ent://...` against an external `postgres://` dev-url | The Atlas image does not ship a Go toolchain; the Ent loader Atlas invokes for `ent://` is a `go run` of the project's own ent code | Wrapper mounts the host Go toolchain read-only at `/usr/local/go` and sets `PATH`/`GOROOT`/`GOCACHE`/`GOMODCACHE` accordingly. The CI `atlas-drift` job runs `actions/setup-go` for the same reason. |
| `ent://` reported as unsupported on `1.2.0-community` / `1.2.0-community-alpine` | community variants drop the ent loader path | The wrapper pins the default `arigaio/atlas:1.2.0` variant, which has been verified to resolve `ent://...` when paired with a mounted host Go toolchain. |

If any of these symptoms recurs, do **not** edit the wrapper to "work
around" it; capture the exact stderr and feed it back so the wrapper
contract covers it. We do not re-implement Atlas semantics in shell.

If the smoke gate ever fails on a future Atlas image bump, the
escalation path is: pinned variant -> next pinned variant in the same
`major.minor.patch` family -> fall back to the GitHub Action
(`ariga/setup-atlas@<40-char-sha>` in CI; pinned binary install on the
host) per `docs/design/DEPENDENCIES.md`.

## Rules

1. Ent schema changes must generate reviewed migrations.
2. Destructive migrations require a migration note and rollback plan.
3. Generated Ent code must be committed with schema changes
   (enforced by `make ent-fresh`).
4. The committed migrations and the live ent schema must never drift
   (enforced by `make atlas-drift`).
5. Tests must use PostgreSQL, not SQLite, once integration tests exist
   (enforced by `make forbidden-sqlite`).
6. The Atlas image tag is pinned to a concrete `major.minor.patch`;
   `latest` and rolling tags are forbidden
   (`docs/design/DEPENDENCIES.md` "Atlas CLI Integration Policy").
