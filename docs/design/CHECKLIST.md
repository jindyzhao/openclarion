# Delivery Checklist

## Foundation

- [ ] license and governance files exist
- [ ] CI workflow exists
- [ ] ADR index is current
- [ ] documentation contains no non-English governed text

## Backend

- [ ] Go module initialized
- [ ] health endpoint exists
- [ ] provider interfaces compile
- [ ] fake providers support tests
- [ ] PostgreSQL migrations run locally

## Control Plane

- [ ] active alerts can be read
- [ ] alert windows can be replayed
- [ ] grouping is deterministic
- [ ] `EvidenceSnapshot` records are persisted
- [ ] Temporal workflow starts from a snapshot

## AI Report Loop

- [ ] `LLMProvider` interface exists
- [ ] JSON report parser validates output
- [ ] golden prompt tests pass
- [ ] failed AI output is marked and retryable

## OpenClaw Sandbox

- [ ] non-root sandbox user
- [ ] fixed timeout
- [ ] network allowlist
- [ ] readonly skills
- [ ] cleanup on success, failure, and timeout

## Frontend

- [ ] report viewer uses generated API types
- [ ] route pages remain thin
- [ ] diagnosis room is tracked as later work
