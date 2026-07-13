FROM scratch

COPY diagnosis-assistant-runner /diagnosis-assistant-runner
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY Apache-2.0.txt /licenses/Apache-2.0.txt
COPY THIRD_PARTY_NOTICES.txt /licenses/THIRD_PARTY_NOTICES.txt
COPY third_party_licenses /licenses/third_party

USER 65532:65532
WORKDIR /workspace
ENTRYPOINT ["/diagnosis-assistant-runner"]
