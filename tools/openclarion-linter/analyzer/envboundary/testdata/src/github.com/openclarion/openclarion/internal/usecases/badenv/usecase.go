package badenv

import env "os"

func mutate() {
	_ = env.Setenv("OPENCLARION_CONFIG", "test") // want "core domain/usecase code must not read or mutate process environment directly"
}
