# Security Policy

## Supported Versions

OpenClarion is in private incubation. Security handling is active for `main` and
any release branch created from it.

| Version | Supported |
|---------|-----------|
| `main` | yes |
| pre-release tags | best effort |

## Reporting a Vulnerability

Do not open a public issue for vulnerabilities after the repository becomes
public. Use a private security advisory or contact the maintainers directly.

Include:

1. description of the vulnerability
2. reproduction steps
3. affected version or commit
4. impact assessment
5. suggested fix, if available

## Response Targets

| Stage | Target |
|-------|--------|
| initial response | within 48 hours |
| severity assessment | within 7 days |
| fix plan | based on severity |
| disclosure | after a fix is available |

## Severity Levels

| Level | Examples | Response |
|-------|----------|----------|
| Critical | remote code execution, credential exposure | immediate fix |
| High | auth bypass, privilege escalation | patch within 7 days |
| Medium | information disclosure, denial of service | next release |
| Low | low-impact hardening issue | roadmap |

## Security Baseline

OpenClarion security work is governed by these defaults:

- authentication and authorization are required for all non-public APIs
- Provider implementations must fail closed
- secrets must not be committed or logged
- candidate agent runtimes must run as short-lived, non-root, restricted
  containers
- AI skills are readonly by default
- production-impacting actions require human approval
- audit logs must be append-only from the application perspective
- generated API contracts must be validated in CI

## Operator Recommendations

1. Use TLS for all external endpoints.
2. Rotate credentials regularly.
3. Enable audit logging.
4. Apply network restrictions around agent sandboxes.
5. Keep dependencies and container images updated.
6. Avoid mounting host secrets into AI containers.
