# Coding Style

## Go

| Rule | Requirement |
|------|-------------|
| formatting | `gofmt` and `goimports` |
| errors | wrap with context and handle all returned errors |
| logging | use structured `slog` |
| function size | prefer small functions with early returns |
| concurrency | use worker pools, errgroup with ownership, or Temporal activities |
| dependencies | inject manually through constructors |
| HTTP | std `net/http` with oapi-codegen `ServerInterface` |

## HTTP Server Pattern

oapi-codegen-exp generates a `ServerInterface` from `api/openapi.yaml`. Handler
implementations satisfy the generated interface. No third-party HTTP framework is
used.

```go
// Generated interface (do not edit)
type ServerInterface interface {
    // (GET /healthz)
    GetHealthz(w http.ResponseWriter, r *http.Request)
}

// Application handler implements the generated interface
type Server struct { /* dependencies */ }

func (s *Server) GetHealthz(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

Note: oapi-codegen-exp V3 does not currently generate a `StrictServerInterface`.
If strict typed request/response wrappers are desired, implement a thin adapter
layer manually or contribute upstream. Accept the V3 `ServerInterface` for MVP.

Middleware uses standard `func(http.Handler) http.Handler` chains.

## Import Ordering

```go
import (
    "context"
    "log/slog"

    "github.com/google/uuid"

    "github.com/openclarion/openclarion/internal/domain"
)
```

## Prohibited Runtime Patterns

- unmanaged goroutines
- hardcoded secrets
- raw SQL string concatenation
- direct external-system calls inside database transactions
- AI-generated output persisted without validation
- placeholder route pages or stub handlers in runtime paths
- importing third-party HTTP frameworks (Gin, Echo, Fiber) without an ADR
- hand-written duplicate DTOs when generated types exist

## Comments

Use comments to explain non-obvious invariants. Avoid comments that repeat the
code. TODO comments must reference an issue.
