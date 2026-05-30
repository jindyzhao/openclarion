package badcontext

import "context"

func run() context.Context {
	return context.Background() // want "core domain/usecase code must not create root or detached contexts"
}
