# OpenClarion Deployment Helpers

This directory contains operator-owned deployment helpers. Tracked files must
never contain real tokens, webhook URLs, registry credentials, database URLs, or
internal endpoint values.

## Service Image

Use the service image helper when a target environment needs an immutable
OpenClarion API/worker image:

```bash
OPENCLARION_SERVICE_IMAGE_REF=harbor.example.test/openclarion/openclarion:20260619-gitsha \
  make openclarion-service-image-build
```

To publish after `docker login` has already been completed outside the repo:

```bash
OPENCLARION_SERVICE_IMAGE_REF=harbor.example.test/openclarion/openclarion:20260619-gitsha \
OPENCLARION_SERVICE_IMAGE_DIGEST_REF_OUT=.openclarion-private/openclarion-service.digest-ref \
OPENCLARION_SERVICE_IMAGE_PROOF_OUT=.openclarion-private/openclarion-service.proof.json \
  make openclarion-service-image-push
```

The helper expects Docker image reference syntax, not URL syntax. Use
`harbor.example.test/openclarion/openclarion:tag`, not
`https://harbor.example.test/openclarion/openclarion:tag`.

Deploy from the printed or retained immutable
`repository@sha256:<digest>` reference, not from the mutable tag used during
build and push. The optional proof JSON binds the mutable tag, immutable digest
reference, local RepoDigest, and remote manifest metadata without retaining
registry credentials.

When the digest or proof output path is inside this repository, the helper
requires it to live under the ignored `.openclarion-private/` directory. Public
repository paths are rejected because retained image proofs can contain internal
registry names even though they do not contain credentials.

## Single-Host Systemd

The `systemd/` subdirectory contains a rehearsal-oriented unit for running the
current checkout plus a prebuilt service binary on one host. It keeps real
runtime values in an operator-owned env file and runs the Stage 5 local worker
readiness check before startup.
