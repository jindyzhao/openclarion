package goodexectest

import (
	"context"
	"os/exec"
)

func run(ctx context.Context) {
	_ = exec.CommandContext(ctx, "sh", "-c", "echo allowed in tests")
}
