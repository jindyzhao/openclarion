package goroutineallowed

func start(fn func()) {
	go fn()
}
