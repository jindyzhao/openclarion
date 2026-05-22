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

## Comments

Use comments to explain non-obvious invariants. Avoid comments that repeat the
code. TODO comments must reference an issue.
