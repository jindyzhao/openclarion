FROM scratch

COPY custom-thin-runner /custom-thin-runner
COPY agent_tool_metric_query /tools/agent_tool_metric_query
COPY agent_tool_topology_lookup /tools/agent_tool_topology_lookup

USER 65532:65532
WORKDIR /workspace
ENTRYPOINT ["/custom-thin-runner"]
