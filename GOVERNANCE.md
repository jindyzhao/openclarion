# Governance

This document describes the governance model for OpenClarion.

## Principles

OpenClarion follows these principles:

- **Open by default**: the project uses the Apache-2.0 license and prepares all
  governance documents before public launch.
- **Transparent decisions**: significant technical choices are recorded as ADRs.
- **Merit-based responsibility**: sustained, high-quality contribution leads to
  broader review and maintainer responsibility.
- **Security-first operations**: identity, approval, sandboxing, and auditability
  are treated as first-class design constraints.
- **Welcoming collaboration**: contributors are expected to follow the Code of
  Conduct and keep discussions technical and respectful.

## Project Roles

### Contributors

Anyone who contributes code, documentation, design feedback, issues, reviews, or
operational experience is a contributor.

Responsibilities:

- follow [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- follow [CONTRIBUTING.md](CONTRIBUTING.md)
- respect active ADRs and design specifications
- keep pull requests focused and reviewable

### Maintainers

Maintainers have write access and are responsible for:

- reviewing and merging pull requests
- triaging issues
- participating in ADR review and acceptance
- mentoring contributors
- protecting repository quality, security, and release integrity

Current maintainers are listed in [MAINTAINERS.md](MAINTAINERS.md).

### Becoming a Maintainer

A contributor may be nominated after demonstrating:

1. sustained contribution over at least three months
2. multiple high-quality merged pull requests or equivalent documentation/design work
3. constructive issue and review participation
4. working knowledge of the architecture and ADRs

Process:

1. an existing maintainer nominates the contributor
2. a one-week discussion period follows
3. lazy consensus applies unless an objection is raised
4. if needed, active maintainers vote and a majority decides

## Decision Making

| Change type | Requirement |
|-------------|-------------|
| small fixes and documentation updates | one maintainer approval |
| new features | one maintainer approval, two preferred |
| dependency updates | compatibility verification and one approval |
| architectural changes | ADR review and maintainer consensus |
| breaking changes | ADR, migration notes, and explicit maintainer approval |

## Architecture Decisions

Significant decisions require an ADR:

1. create an ADR using `docs/adr/TEMPLATE.md`
2. keep it in `Proposed` status for review
3. record consequences and confirmation checks
4. accept, reject, amend, or supersede through follow-up ADR lifecycle

## Conflict Resolution

1. technical disagreements are resolved in the relevant issue or pull request
2. interpersonal conflicts follow the Code of Conduct process
3. unresolved technical disputes may be decided by maintainer vote

## Communication

While the repository is private, discussion happens in repository issues and pull
requests. Public issue and discussion channels will be enabled when the project
moves to a public repository.

## Governance Changes

Changes to this document require a pull request, at least a one-week review
period, and approval by active maintainers.
