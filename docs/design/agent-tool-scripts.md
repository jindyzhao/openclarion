# Agent Tool Scripts

OpenClarion's current product direction remains intelligent alert analysis.
M4 sandbox tooling exists to help a sandboxed runtime inspect prepared alert
evidence, metrics, and topology without giving the agent process broad host
access.

The first tool helpers are small Go commands under `scripts/`:

| Tool | Command | Purpose |
|------|---------|---------|
| metric query | `agent_tool_metric_query` | read-only Prometheus instant query through `/api/v1/query` |
| topology lookup | `agent_tool_topology_lookup` | read-only static JSON service-topology lookup |

They are not Go control-plane provider implementations. They are candidate
runtime image contents: OpenClaw, Hermes Agent, or custom runner images can
include the compiled binaries and call them as local tools inside the sandbox.
The control plane still owns provider configuration, credentials, network
policy, lifecycle, output validation, and persistence.

`make custom-thin-runner-smoke` packages these helpers into the local custom
thin runner image and proves the topology helper inside the digest-pinned
candidate image using `docker run --entrypoint`.

## Contract

Every tool helper follows the same operator-facing rules:

- read configuration from flags and environment variables;
- never accept secrets as CLI flags;
- perform one bounded read-only action;
- write exactly one JSON object to stdout on success;
- write diagnostics to stderr on failure;
- exit non-zero on invalid input, upstream error, malformed response, or an
  output contract violation.

The JSON object shape is intentionally stable enough for later schema tests but
small enough to avoid locking in a full agent protocol.

## Metric Query Helper

`scripts/agent_tool_metric_query` calls Prometheus's instant query endpoint.
The implementation follows Prometheus's documented API envelope:
`status`, `data.resultType`, `data.result`, optional `warnings`, and optional
`infos`. The response envelope and nested query `data` object are parsed as
strict JSON, so duplicate object keys, unknown fields, and trailing JSON values
are rejected before helper output is emitted.

Configuration:

| Input | Required | Notes |
|-------|----------|-------|
| `--prometheus-url` or `OPENCLARION_TOOL_PROMETHEUS_URL` | yes | base URL; must be `http` or `https` with no userinfo |
| `--query` | yes | PromQL instant query |
| `--time` | no | RFC3339 or unix timestamp passed through to Prometheus |
| `--query-timeout` | no | Prometheus query timeout parameter; default `10s` |
| `--http-timeout` | no | client-side timeout; default `10s` |
| `--limit` | no | returned-series limit; default `100`, max `10000` |
| `OPENCLARION_TOOL_PROMETHEUS_BEARER_TOKEN` | no | short-lived token injected by the control plane |

Success output:

```json
{
  "tool": "metric_query",
  "source": "prometheus",
  "query": "up",
  "result_type": "vector",
  "result": []
}
```

## Topology Lookup Helper

`scripts/agent_tool_topology_lookup` reads a bounded static JSON file. This is
the first CMDB-shaped helper, not the final CMDB provider. The mounted topology
file must be a regular file; symlinks and other non-regular files are rejected
before the helper opens or parses JSON. YAML, NetBox, or HTTP-backed topology
providers remain future work.

Configuration:

| Input | Required | Notes |
|-------|----------|-------|
| `--topology-file` or `OPENCLARION_TOOL_TOPOLOGY_FILE` | yes | static JSON file mounted readonly |
| `--service` | yes | plain service name; path separators are rejected |

Input file:

```json
{
  "services": [
    {
      "name": "payments",
      "owner": "payments-team",
      "tier": "backend",
      "dependencies": ["postgres"],
      "dependents": ["checkout"],
      "runbooks": ["runbook:payments"],
      "metadata": { "env": "prod" }
    }
  ]
}
```

Success output includes the selected node plus any dependency/dependent nodes
present in the same file.

Runbook values in static topology examples are internal identifiers, not live
external documentation links. Real deployments can map them to owned runbook
systems before mounting the topology file.

## Security Posture

- The helpers are read-only and do not write files.
- Metric access depends on the sandbox egress allowlist; the helper itself does
  not bypass Docker network policy.
- Bearer tokens are read only from environment variables, matching the
  short-lived credential boundary.
- Responses and topology files are capped at 4 MiB to avoid unbounded memory
  reads.
- Prometheus response envelopes and nested query data are parsed as strict JSON:
  duplicate object keys, unknown fields, and trailing JSON values are rejected.
- The topology file is parsed as strict JSON: duplicate object keys, unknown
  fields, and trailing JSON values are rejected before lookup output is emitted.
- Tests run through `make agent-tool-scripts-test`; the full `make ci` bundle
  also executes them.
