package goodgroup

type group struct{}

func (group) Go(func() error) {}

func run() {
	var g group
	g.Go(func() error {
		return nil
	})
}
