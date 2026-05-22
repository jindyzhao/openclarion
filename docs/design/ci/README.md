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

| Gate | Introduced At | Notes |
|------|---------------|-------|
| docs hygiene (English-only check) | M0 | already active |
| ADR / phase doc link validation | M0 | added with the bootstrap PR |
| Go module: `go vet`, `go build`, `go test ./...` | M0 | minimal once Go skeleton lands |
| OpenAPI lint (e.g. `redocly lint`) | M0 | with healthz spec |
| OpenAPI generation freshness (`make generate` no diff) | M0 | enforces contract-first |
| `oapi-codegen-exp` commit-hash pin check | M0 | rejects `latest` for first-party deps |
| Ent generation freshness | M1 | once Ent schema exists |
| Atlas migration drift check | M1 | once first migration lands |
| Temporal workflow tests (replay, signal, timer) | M1 | once first workflow lands |
| Provider boundary lint (forbidden imports) | M2 | once provider layer is real |
| LLM golden prompt structural tests | M2 | once LLMProvider exists |
| LLM refusal / truncation handling tests | M2 | covers Structured Outputs failure modes |
| Frontend typecheck and unit tests | M3 | once `web/` lands |
| OpenAPI -> TS type freshness | M3 | enforces frontend contract sync |
| OpenTelemetry trace integration smoke | M3 | observability check |
| Container sandbox security gate (non-root, limits) | M4 | once ContainerProvider lands |
| WebSocket auth handshake test | M5 | once diagnosis room lands |
| Bounded-turn enforcement test | M5 | guards against client-side bypass |
| Audit completeness test | M5 | every lifecycle event logged |

## Current Private-Incubation Gate

The initial workflow runs the documentation language gate:

```bash
bash scripts/check_no_non_english_chars.sh
```

Additional gates land per the schedule above as code is introduced.

## Workflow Policy

- GitHub Actions must call repository-owned scripts or `make` targets.
- Do not duplicate long inline command lists in workflow YAML.
- Generated-code checks must fail on dirty working tree.
- Allowlist files must have owners and expiration criteria.
- A new gate must ship with documentation in this file before being merged.
- Gates that block PRs must run within 10 minutes wall-clock for M0-M2.
