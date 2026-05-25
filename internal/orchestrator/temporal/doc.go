// Package temporal contains the Temporal-backed orchestrator
// implementation. Layering rules enforced by CI:
//
//   - MAY import go.temporal.io/sdk/..., internal/domain,
//     internal/usecases/ports
//   - MUST NOT import internal/persistence/ent, internal/transport,
//     or any provider package
//
// Workflow functions are deterministic and MUST NOT perform I/O.
// Activities hold all I/O (database writes, external calls) and
// receive dependencies via the Activities struct.
package temporal
