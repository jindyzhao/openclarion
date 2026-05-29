package badlog

import (
	"fmt"
	"log"
)

func run() {
	fmt.Println("debug")             // want "production code must use structured logging instead of fmt.Println"
	fmt.Printf("debug: %s", "value") // want "production code must use structured logging instead of fmt.Printf"
	log.Println("debug")             // want "production code must use structured logging instead of log.Println"
	log.Fatalf("fatal: %s", "value") // want "production code must use structured logging instead of log.Fatalf"
}
