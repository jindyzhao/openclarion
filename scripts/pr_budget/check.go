// Command pr_budget runs a child validation command and enforces the
// OpenClarion local PR wall-clock budget.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultBudget = 15 * time.Minute
	exitUsage     = 2
	exitBudget    = 124
)

type budgetMode string

const (
	modeEnforce budgetMode = "enforce"
	modeWarn    budgetMode = "warn"
)

type config struct {
	Budget  time.Duration
	Mode    budgetMode
	Command []string
}

type commandRunner interface {
	Run(ctx context.Context, command []string, stdout, stderr io.Writer) (int, error)
}

type realRunner struct{}

type clock interface {
	Now() time.Time
}

type realClock struct{}

func main() {
	code := run(os.Args[1:], realRunner{}, realClock{}, os.Getenv, os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(args []string, runner commandRunner, clk clock, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-budget] %v\n", err)
		return exitUsage
	}

	fmt.Fprintf(stderr, "[pr-budget] running %s with budget %s (mode=%s)\n", strings.Join(cfg.Command, " "), cfg.Budget, cfg.Mode)
	ctx := context.Background()
	cancel := func() {}
	if cfg.Mode == modeEnforce {
		ctx, cancel = context.WithTimeout(ctx, cfg.Budget)
	}
	defer cancel()

	started := clk.Now()
	exitCode, err := runner.Run(ctx, cfg.Command, stdout, stderr)
	elapsed := clk.Now().Sub(started)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		msg := fmt.Sprintf("[pr-budget] elapsed %s exceeded budget %s", roundDuration(elapsed), cfg.Budget)
		fmt.Fprintln(stderr, msg)
		return exitBudget
	}
	if err != nil {
		if exitCode == 0 {
			exitCode = 1
		}
		fmt.Fprintf(stderr, "[pr-budget] command failed after %s: %v\n", roundDuration(elapsed), err)
		return exitCode
	}
	if exitCode != 0 {
		fmt.Fprintf(stderr, "[pr-budget] command exited with code %d after %s\n", exitCode, roundDuration(elapsed))
		return exitCode
	}

	if elapsed > cfg.Budget {
		msg := fmt.Sprintf("[pr-budget] elapsed %s exceeded budget %s", roundDuration(elapsed), cfg.Budget)
		if cfg.Mode == modeWarn {
			fmt.Fprintf(stderr, "%s (warn-only)\n", msg)
			return 0
		}
		fmt.Fprintln(stderr, msg)
		return exitBudget
	}

	fmt.Fprintf(stderr, "[pr-budget] OK elapsed=%s budget=%s\n", roundDuration(elapsed), cfg.Budget)
	return 0
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	defaultBudgetText := strings.TrimSpace(getenv("OPENCLARION_PR_BUDGET"))
	if defaultBudgetText == "" {
		defaultBudgetText = defaultBudget.String()
	}
	defaultModeText := strings.TrimSpace(getenv("OPENCLARION_PR_BUDGET_MODE"))
	if defaultModeText == "" {
		defaultModeText = string(modeEnforce)
	}

	fs := flag.NewFlagSet("pr_budget", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var budgetText string
	var modeText string
	fs.StringVar(&budgetText, "budget", defaultBudgetText, "wall-clock budget for the child command, e.g. 15m")
	fs.StringVar(&modeText, "mode", defaultModeText, "budget mode: enforce or warn")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	budget, err := time.ParseDuration(budgetText)
	if err != nil {
		return config{}, fmt.Errorf("--budget must be a Go duration: %w", err)
	}
	if budget <= 0 {
		return config{}, errors.New("--budget must be greater than zero")
	}

	mode := budgetMode(modeText)
	switch mode {
	case modeEnforce, modeWarn:
	default:
		return config{}, fmt.Errorf("--mode must be %q or %q", modeEnforce, modeWarn)
	}

	command := fs.Args()
	if len(command) == 0 {
		return config{}, errors.New("usage: pr_budget [--budget 15m] [--mode enforce|warn] -- <command> [args...]")
	}
	return config{
		Budget:  budget,
		Mode:    mode,
		Command: command,
	}, nil
}

func (realRunner) Run(ctx context.Context, command []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...) // #nosec G204 -- command comes from the repository-owned Makefile target.
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}

func (realClock) Now() time.Time {
	return time.Now()
}

func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Millisecond)
}
