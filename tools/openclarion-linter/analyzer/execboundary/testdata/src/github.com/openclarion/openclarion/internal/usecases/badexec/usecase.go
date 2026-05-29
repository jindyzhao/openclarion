package badexec

import (
	"context"
	"os/exec"
)

func run(ctx context.Context) {
	_ = exec.Command("sh", "-c", "echo bad")             // want "production code must not call os/exec.Command outside cmd, scripts, or the sandbox boundary"
	_ = exec.CommandContext(ctx, "sh", "-c", "echo bad") // want "production code must not call os/exec.CommandContext outside cmd, scripts, or the sandbox boundary"
	localExec := struct{ Command func(string, ...string) any }{}
	_ = localExec.Command("not-os-exec")
}
