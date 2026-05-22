# CI Governance

CI is a product quality boundary, not only a build runner. Required checks are
owned by repository scripts or `make` targets so local and remote validation stay
aligned.

## Gate Families

| Gate | Purpose |
|------|---------|
| docs hygiene | no non-English governed docs, valid links |
| generated code | OpenAPI, Ent, and frontend type freshness |
| backend tests | Go unit and integration tests |
| frontend tests | typecheck, unit tests, smoke tests |
| security | secret scan, vulnerability scan, dependency audit |
| architecture | provider boundaries, transaction boundaries, no unmanaged goroutines |

## Current Private-Incubation Gate

The initial workflow runs the documentation language gate:

```bash
bash scripts/check_no_non_english_chars.sh
```

Additional gates are added as the code skeleton lands.

## Workflow Policy

- GitHub Actions must call repository-owned scripts or `make` targets.
- Do not duplicate long inline command lists in workflow YAML.
- Generated-code checks must fail on dirty working tree.
- Allowlist files must have owners and expiration criteria.
