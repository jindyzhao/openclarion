package goodenv

type Config struct {
	Endpoint string
}

func endpoint(cfg Config) string {
	return cfg.Endpoint
}
