package badcontext

import ctx "context"

func run() ctx.Context {
	return ctx.TODO() // want "core domain/usecase code must not create root or detached contexts"
}

func detach(parent ctx.Context) ctx.Context {
	return ctx.WithoutCancel(parent) // want "core domain/usecase code must not create root or detached contexts"
}
