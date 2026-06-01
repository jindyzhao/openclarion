package contextallowed

import "context"

func fallback() context.Context {
	return context.Background()
}
