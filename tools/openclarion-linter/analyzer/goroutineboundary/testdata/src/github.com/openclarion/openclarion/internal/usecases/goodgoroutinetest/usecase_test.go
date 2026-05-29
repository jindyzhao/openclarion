package goodgoroutinetest

func run(fn func()) {
	go fn()
}
