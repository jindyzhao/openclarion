# Roadmap

> Last updated: 2026-05-18  
> Author: jindyzhao  
> Status: private incubation

## Milestones

```text
M0 Bootstrap  ->  M1 Go Control Plane  ->  M2 Headless AI Reports  ->  M3 Interactive Diagnosis
```

## M0: Bootstrap

- [ ] governance files
- [ ] GitHub issue and PR templates
- [ ] CI documentation hygiene check
- [ ] Go module skeleton
- [ ] PostgreSQL and Temporal local stack
- [ ] OpenAPI 3.1 skeleton
- [ ] Ent and Atlas toolchain

## M1: Go Control Plane

- [ ] MetricsProvider for active alerts
- [ ] AlertEvent schema
- [ ] alert window replay harness
- [ ] deterministic grouping
- [ ] EvidenceSnapshot schema and builder
- [ ] Temporal workflow bootstrap
- [ ] Email, Webhook, and Slack providers

## M2: Headless AI Reports

- [ ] LLMProvider interface
- [ ] mock provider
- [ ] OpenAI-compatible provider
- [ ] prompt templates
- [ ] structured JSON output parser
- [ ] SubReport and FinalReport schemas
- [ ] golden prompt tests
- [ ] report notification flow
- [ ] OpenClaw headless sandbox PoC

## M3: Interactive Diagnosis

- [ ] report viewer frontend
- [ ] diagnosis room route
- [ ] AuthProvider integration
- [ ] RBAC checks
- [ ] WebSocket proxy
- [ ] ChatSession and ChatTurn schemas
- [ ] lifecycle-end compression
- [ ] audit and unsafe-instruction filtering

## Future

- [ ] pgvector retrieval
- [ ] Kubernetes Job ContainerProvider
- [ ] DingTalk and Feishu providers
- [ ] NetBox provider
- [ ] scheduled weekly and monthly reports
- [ ] multi-tenant operations

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | English roadmap reset and MVP cutline |
