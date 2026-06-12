# OpenClarion Stage 5 systemd Unit

This directory contains a single-host systemd template for running the current
OpenClarion API and worker with the Stage 5 diagnosis sandbox wiring. It is for
operator rehearsal and small single-host deployments where Docker remains the
`ContainerProvider` backend.

The unit keeps secrets in an operator-owned env file and uses the existing
`make stage5-local-worker-check` readiness target before starting the service.
It expects a prebuilt OpenClarion binary instead of running `go run` under
systemd.

## Layout

```text
/opt/openclarion/current        source checkout or release tree
/opt/openclarion/bin/openclarion prebuilt OpenClarion binary
/etc/openclarion/openclarion-stage5.env private env file, mode 0600
/etc/systemd/system/openclarion-stage5.service installed unit
```

## Install

Create the service user and prepare directories:

```bash
sudo useradd --system --home-dir /var/lib/openclarion --create-home --shell /usr/sbin/nologin openclarion
sudo usermod -aG docker openclarion
sudo install -d -o openclarion -g openclarion -m 0750 /opt/openclarion/bin
sudo install -d -o openclarion -g openclarion -m 0750 /etc/openclarion
```

Build and install the binary from the checked-out tree. The build target writes
to an ignored path by default and also writes a sibling SHA-256 file:

```bash
make openclarion-release-build
sudo install -o openclarion -g openclarion -m 0750 \
  .openclarion-private/release/openclarion \
  /opt/openclarion/bin/openclarion
```

Install the private env file from an ignored local proof env. Do not place real
values in tracked files:

```bash
sudo install -o openclarion -g openclarion -m 0600 \
  .openclarion-private/live.env \
  /etc/openclarion/openclarion-stage5.env
```

Install and enable the unit:

```bash
sudo install -o root -g root -m 0644 \
  deploy/systemd/openclarion-stage5.service \
  /etc/systemd/system/openclarion-stage5.service
sudo systemctl daemon-reload
sudo systemctl enable openclarion-stage5.service
```

Run the same readiness check the unit runs before starting:

```bash
sudo -u openclarion \
  OPENCLARION_STAGE5_WORKER_ENV_FILE=/etc/openclarion/openclarion-stage5.env \
  OPENCLARION_STAGE5_WORKER_BINARY=/opt/openclarion/bin/openclarion \
  /opt/openclarion/current/scripts/run_stage5_local_worker.sh --check-only
```

Start and inspect:

```bash
sudo systemctl start openclarion-stage5.service
sudo systemctl status openclarion-stage5.service
journalctl -u openclarion-stage5.service -f
```

## Runtime Notes

- The service user must be able to access the Docker daemon through the local
  deployment's approved mechanism, usually the `docker` group on a rehearsal
  host.
- The private env file must remain a direct, non-symlink file owned by the
  service user with no group or world permissions.
- `OPENCLARION_SANDBOX_IMAGE_REF` must be an immutable digest reference and
  must be locally present or pullable by Docker.
- `OPENCLARION_SANDBOX_EGRESS_NETWORK` must already exist before the service is
  started.
- The unit runs with `NoNewPrivileges=yes`, `ProtectSystem=strict`, and related
  hardening. It leaves write access only to temporary directories and the Docker
  socket path needed by the current Docker-backed sandbox provider.
