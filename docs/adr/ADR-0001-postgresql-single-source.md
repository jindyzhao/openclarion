---
id: ADR-0001
title: "PostgreSQL as the Single Source of Truth"
status: "proposed"
date: 2026-05-18
deciders: ["jindyzhao"]
consulted: []
informed: []
---

# ADR-0001: PostgreSQL as the Single Source of Truth

> **Review Period**: Until 2026-05-20 (48-hour minimum)

## Context and Problem Statement

OpenClarion must persist alert evidence, diagnosis tasks, workflow state links,
chat turns, reports, audit records, and future retrieval metadata. The project
needs a data strategy that avoids unnecessary operational surface area during the
first implementation phase.

## Decision Drivers

* keep operational dependencies small
* support relational queries, JSON evidence, and future vector search
* align with Temporal's PostgreSQL deployment path
* keep migrations reviewable
* avoid early Redis, MongoDB, or separate vector database dependency

## Considered Options

* **Option 1**: PostgreSQL only, with JSONB and future pgvector
* **Option 2**: PostgreSQL plus MongoDB
* **Option 3**: PostgreSQL plus Redis and a vector database

## Decision Outcome

**Chosen option**: "PostgreSQL only", because it provides relational integrity,
JSONB flexibility, migration discipline, and future vector-search capability with
minimum operational complexity.

### Consequences

* Good, because one durable database holds all business records.
* Good, because Ent and Atlas can govern schema changes.
* Good, because JSONB can preserve raw alert and tool evidence.
* Neutral, because very high-throughput caches may need a later ADR.
* Bad, because some specialized retrieval workloads may eventually need tuning
  or a dedicated system.

### Confirmation

* `go.mod` does not import Redis or MongoDB clients for core runtime paths.
* database design docs define Ent schemas for alert, task, snapshot, chat, and report records.
* CI blocks SQLite usage in tests once database tests exist.

## More Information

### Implementation Notes

Use one PostgreSQL cluster with separate databases or schemas for Temporal and
OpenClarion business data. Do not share application tables with Temporal system
tables.

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-05-18 | jindyzhao | Initial proposal |
