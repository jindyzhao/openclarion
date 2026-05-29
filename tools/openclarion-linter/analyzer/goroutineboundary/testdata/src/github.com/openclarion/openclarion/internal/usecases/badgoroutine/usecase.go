package badgoroutine

func run(ch chan<- string) {
	go func() { // want "production code must not start goroutines with raw go statements"
		ch <- "bad"
	}()
}
