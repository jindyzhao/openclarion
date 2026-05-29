package badpanic

func run() {
	panic("boom") // want "production code must not call panic outside main, init, or an explicit recover boundary"
}

func shadowed() {
	panic := func(any) {}
	panic("not the builtin")
}
