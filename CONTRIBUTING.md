# Contributing to OpenClarion

Thank you for contributing to OpenClarion. This repository is private during
incubation, but it follows the same contribution discipline expected from a
public open source project.

## Code of Conduct

All contributors must follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Issue Guidelines

Good issues are small, actionable, and independently reviewable.

| Principle | Requirement |
|-----------|-------------|
| one concern | one bug, feature, or doc topic per issue |
| clear context | current behavior, expected behavior, impact |
| acceptance criteria | testable completion criteria |
| traceability | link ADRs, design docs, or roadmap items when relevant |

## Pull Request Guidelines

1. Create a branch from the latest `main`.
2. Keep the PR atomic and self-contained.
3. Update tests, docs, ADRs, or design specs in the same PR when they are part of
   the same concern.
4. Run local validation before requesting review.
5. Sign off every commit with DCO.

Recommended local validation:

```bash
make generate
make lint
make test
make pr
```

If only documentation changed, run:

```bash
bash scripts/check_no_non_english_chars.sh
```

## Atomic PRs

| Scenario | Action |
|----------|--------|
| bug fix plus unrelated refactor | split into two PRs |
| feature plus its own tests | one PR |
| API contract plus generated code | one PR |
| ADR proposal plus implementation | split unless the ADR is already accepted |

## Commit Messages

Use Conventional Commits with DCO sign-off:

```text
feat(provider): add alertmanager provider contract

Adds the provider interface and fake implementation used by workflow tests.

Refs #123
Signed-off-by: Your Name <you@example.com>
```

Allowed types:

`feat`, `fix`, `docs`, `refactor`, `test`, `ci`, `chore`, `perf`, `style`

## Development Standards

- Go code follows [docs/design/CODING_STYLE.md](docs/design/CODING_STYLE.md).
- Security-sensitive code follows [docs/design/SECURITY_CODING.md](docs/design/SECURITY_CODING.md).
- API changes follow [ADR-0007](docs/adr/ADR-0007-openapi-31-native-toolchain.md).
- Frontend work follows [docs/design/frontend/README.md](docs/design/frontend/README.md).
- CI policy follows [docs/design/ci/README.md](docs/design/ci/README.md).

## DCO

Every commit must include a sign-off line:

```bash
git commit -s
```

See [DCO.md](DCO.md) for details.
