package main

import "os"

func readAtBoundary(path string) ([]byte, error) {
	return os.ReadFile(path)
}
