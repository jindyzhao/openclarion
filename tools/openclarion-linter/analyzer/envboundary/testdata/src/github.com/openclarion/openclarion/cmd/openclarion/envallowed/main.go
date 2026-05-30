package main

import "os"

func readAtBoundary() string {
	return os.Getenv("OPENCLARION_CONFIG")
}
