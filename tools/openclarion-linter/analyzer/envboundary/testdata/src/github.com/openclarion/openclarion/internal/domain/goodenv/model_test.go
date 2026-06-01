package goodenv

import "os"

func fixtureEndpoint() string {
	return os.Getenv("OPENCLARION_CONFIG")
}
