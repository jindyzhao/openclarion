package badenv

import "os"

func read() string {
	return os.Getenv("OPENCLARION_CONFIG") // want "core domain/usecase code must not read or mutate process environment directly"
}
