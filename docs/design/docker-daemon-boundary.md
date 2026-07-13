# Docker Daemon Access Boundary

OpenClarion's current product direction remains intelligent alert analysis.
This document only scopes the M4/M5 sandbox runtime boundary: how the Go
control plane is allowed to reach Docker Engine when it creates short-lived
agent containers. The sandbox file, network, and lifecycle contract originates
in [ADR-0013](../adr/ADR-0013-per-turn-container-invocation.md).

## V1 Boundary

V1 may use the local host Docker daemon so the control plane can run the
Docker-backed `ContainerProvider` without introducing a second scheduler.
This is a privileged boundary:

- The Docker daemon can create containers with host-affecting privileges.
- Access to `/var/run/docker.sock` must be treated as host-admin equivalent.
- The Docker socket must never be mounted into an agent sandbox container.
- Only the Go control plane process may call Docker Engine.
- Sandbox containers remain non-root, readonly-rootfs, capability-dropped,
  resource-limited, and short-lived.

The sandbox security spec and smoke harness already reject Docker socket bind
mounts. That protects the container interior from reaching the daemon directly;
it does not remove the host-side privilege held by the control plane.

## Runtime Rules

For the V1 host-socket mode:

- Run OpenClarion on a dedicated sandbox-capable host or VM when possible.
- Keep the control plane under a dedicated operating-system user or service
  account.
- Do not expose the Docker API over plaintext TCP.
- Do not share the daemon with unrelated workloads that require different
  trust boundaries.
- Keep candidate agent runtime dependencies inside digest-pinned sandbox
  images, not in the Go control plane.
- Keep sandbox egress fail-closed unless the configured egress enforcer proves
  the allowlist boundary.
- Run the bundled allowlist proxy only on an operator-owned internal Docker
  network without a published host port. It is not an authenticated shared
  forward proxy.
- Attach sandbox containers only to that internal network. The proxy is the
  only service that also joins a network with external connectivity.
- Let the Docker provider own upper- and lower-case HTTP proxy variables,
  clear bypass variables, and reject runtime credentials that attempt to
  override those names.
- Keep allowlisted DNS names under trusted operator or provider ownership;
  exact hostname matching does not protect a target whose DNS is compromised.

If a remote Docker daemon is required, the only accepted direction is a
TLS-verified endpoint with client authentication. Docker's own daemon-access
guidance warns that unsecured remote access can allow unauthorized users to
gain host root-equivalent access. Docker also documents TLS verification with
trusted CA and client certificates for daemon and client connections.

References:

- [Docker: Protect the Docker daemon socket](https://docs.docker.com/engine/security/protect-access/)
- [Docker: Configure remote access for Docker daemon](https://docs.docker.com/engine/daemon/remote-access/)
- [Docker: Configure proxy settings](https://docs.docker.com/engine/cli/proxy/)
- [Docker Compose: Networking](https://docs.docker.com/compose/how-tos/networking/)

## Post-V1 Direction

Post-V1 should reduce the daemon privilege surface before broad deployment:

| Option | Use when | Required proof |
|--------|----------|----------------|
| Rootless Docker | Single-host deployments need a smaller host privilege blast radius | `ContainerProvider.Run` passes the same lifecycle, mount, output-cap, credential, and egress tests under rootless mode |
| Dedicated sandbox host | The main control plane should not share the Docker daemon host | Docker API is reachable only over mutually authenticated TLS; host networking and firewall rules isolate sandbox egress |
| Kubernetes Job provider | Deployment already uses a cluster with namespace and network-policy controls | New `ContainerProvider` implementation preserves ADR-0013 file contract, timeout cleanup, resource limits, and output validation |

None of these options change the current M4/M5 file contract. They are runtime
deployment choices behind `ContainerProvider`.

## Acceptance State

Documented and test-covered now:

- Docker socket mount rejection in the sandbox security spec.
- Docker Engine lifecycle is isolated behind an injectable `EngineClient`.
- Default network-none and allowlist-mode fail-closed behavior.
- Short-lived credential validation before Docker create.
- Live local Docker Engine `ContainerProvider.Run` smoke through
  `make container-provider-smoke`.
- Live local timeout cleanup smoke through
  `make container-provider-timeout-smoke`.
- Live local output cap smoke through
  `make container-provider-output-cap-smoke`.
- Concrete Docker internal-network + proxy allow/deny smoke through
  `make egress-allowdeny-smoke`.
- Bundled exact `host[:port]` HTTP/CONNECT proxy, packaged as a non-root scratch
  image through `make local-egress-proxy-build`; focused tests cover allowed and
  denied targets, CONNECT tunneling, hop-by-hop header removal, health checks,
  and malformed configuration.
- Compose `sandbox-egress` profile with an externally isolated sandbox network,
  a dual-network proxy without a published host port, and local-only image
  pull policy.
- Docker provider fail-closed proxy URL validation, proxy environment
  injection, bypass-variable clearing, credential override rejection, and
  diagnosis LLM target coverage validation before worker startup. Each
  allowlist invocation also inspects the selected Docker network before create
  and requires the exact configured name, `Internal=true`, and a non-ingress,
  non-config-only runtime network.
- Local custom thin runner candidate proof through
  `make custom-thin-runner-smoke`, using a digest-pinned localhost-registry
  image reference through both runtime and Provider harnesses.

Still pending:

- Full Docker provider-path proof using the bundled Eino diagnosis runner.
- Rootless Docker or dedicated sandbox host proof for post-V1.
