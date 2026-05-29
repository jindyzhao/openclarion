package main

import (
	"context"
	"os/exec"
)

func main() {
	_ = exec.CommandContext(context.Background(), "sh", "-c", "echo allowed")
}
