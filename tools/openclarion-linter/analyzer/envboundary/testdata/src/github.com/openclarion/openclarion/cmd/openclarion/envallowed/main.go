package main

import (
	"flag"
	"os"
)

func readAtBoundary() string {
	flag.Parse()
	return os.Getenv("OPENCLARION_CONFIG")
}
