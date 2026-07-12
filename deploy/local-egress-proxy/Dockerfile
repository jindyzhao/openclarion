FROM scratch

COPY openclarion-egress-proxy /openclarion-egress-proxy

USER 65532:65532
ENTRYPOINT ["/openclarion-egress-proxy", "serve"]
