package recoverboundary

func withinRecoverBoundary(fn func()) {
	defer func() {
		if v := recover(); v != nil {
			panic(v)
		}
	}()
	fn()
}
