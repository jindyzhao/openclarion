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

The V1 transport uses the generated `ServerInterface` directly. Do not add a
handwritten strict adapter unless upstream generates one for the pinned OpenAPI
3.1 path or review shows repeated request/response boilerplate that the adapter
would remove without duplicating generated routing or DTO contracts.

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
code. TODO, FIXME, HACK, and XXX comments must reference an issue or ADR.
