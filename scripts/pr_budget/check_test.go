package main

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	exitCode int
	err      error
	elapsed  time.Duration
	clock    *fakeClock
	command  []string
}

func (r *fakeRunner) Run(_ context.Context, command []string, _, _ io.Writer) (int, error) {
	r.command = append([]string(nil), command...)
	r.clock.advance(r.elapsed)
	return r.exitCode, r.err
}

type fakeClock struct {
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 5, 29, 1, 2, 3, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestParseConfigUsesEnvDefaultsAndCommand(t *testing.T) {
	cfg, err := parseConfig([]string{"--", "make", "ci"}, mapEnv(map[string]string{
		"OPENCLARION_PR_BUDGET":      "12m",
		"OPENCLARION_PR_BUDGET_MODE": "warn",
	}))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.Budget != 12*time.Minute {
		t.Fatalf("Budget = %s", cfg.Budget)
	}
	if cfg.Mode != modeWarn {
		t.Fatalf("Mode = %q", cfg.Mode)
	}
	if got := strings.Join(cfg.Command, " "); got != "make ci" {
		t.Fatalf("Command = %q", got)
	}
}

func TestParseConfigRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing command", args: []string{"--budget", "1m"}, want: "usage"},
		{name: "bad budget", args: []string{"--budget", "soon", "--", "make", "ci"}, want: "duration"},
		{name: "zero budget", args: []string{"--budget", "0s", "--", "make", "ci"}, want: "greater than zero"},
		{name: "bad mode", args: []string{"--mode", "skip", "--", "make", "ci"}, want: "mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, mapEnv(nil))
			if err == nil {
				t.Fatal("parseConfig err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseConfig err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRunSucceedsWithinBudget(t *testing.T) {
	clk := newFakeClock()
	runner := &fakeRunner{clock: clk, elapsed: 2 * time.Minute}
	var stderr strings.Builder

	code := run([]string{"--budget", "3m", "--", "make", "ci"}, runner, clk, mapEnv(nil), io.Discard, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, stderr=%s", code, stderr.String())
	}
	if got := strings.Join(runner.command, " "); got != "make ci" {
		t.Fatalf("command = %q", got)
	}
	if !strings.Contains(stderr.String(), "OK elapsed=2m0s budget=3m0s") {
		t.Fatalf("stderr = %q, want OK elapsed", stderr.String())
	}
}

func TestRunFailsWhenSuccessfulCommandExceedsEnforcedBudget(t *testing.T) {
	clk := newFakeClock()
	runner := &fakeRunner{clock: clk, elapsed: 4 * time.Minute}
	var stderr strings.Builder

	code := run([]string{"--budget", "3m", "--", "make", "ci"}, runner, clk, mapEnv(nil), io.Discard, &stderr)

	if code != exitBudget {
		t.Fatalf("run code = %d, want %d; stderr=%s", code, exitBudget, stderr.String())
	}
	if !strings.Contains(stderr.String(), "exceeded budget") {
		t.Fatalf("stderr = %q, want budget failure", stderr.String())
	}
}

func TestRunWarnModeDoesNotFailWhenOverBudget(t *testing.T) {
	clk := newFakeClock()
	runner := &fakeRunner{clock: clk, elapsed: 4 * time.Minute}
	var stderr strings.Builder

	code := run([]string{"--budget", "3m", "--mode", "warn", "--", "make", "ci"}, runner, clk, mapEnv(nil), io.Discard, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warn-only") {
		t.Fatalf("stderr = %q, want warn-only", stderr.String())
	}
}

func TestRunTerminatesCommandWhenEnforcedBudgetExpires(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep command test is Unix-only")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skipf("sleep command unavailable: %v", err)
	}
	var stderr strings.Builder

	started := time.Now()
	code := run([]string{"--budget", "75ms", "--", "sleep", "2"}, realRunner{}, realClock{}, mapEnv(nil), io.Discard, &stderr)
	elapsed := time.Since(started)

	if code != exitBudget {
		t.Fatalf("run code = %d, want %d; stderr=%s", code, exitBudget, stderr.String())
	}
	if elapsed > time.Second {
		t.Fatalf("run elapsed = %s, command was not terminated near budget", elapsed)
	}
	if !strings.Contains(stderr.String(), "exceeded budget") {
		t.Fatalf("stderr = %q, want budget failure", stderr.String())
	}
}

func TestRunPreservesChildExitCodeBeforeBudgetEvaluation(t *testing.T) {
	clk := newFakeClock()
	runner := &fakeRunner{clock: clk, elapsed: 4 * time.Minute, exitCode: 7}
	var stderr strings.Builder

	code := run([]string{"--budget", "3m", "--", "make", "ci"}, runner, clk, mapEnv(nil), io.Discard, &stderr)

	if code != 7 {
		t.Fatalf("run code = %d, want child exit code 7", code)
	}
	if strings.Contains(stderr.String(), "exceeded budget") {
		t.Fatalf("stderr = %q, budget should not mask child failure", stderr.String())
	}
}

func TestRunReportsStartFailure(t *testing.T) {
	clk := newFakeClock()
	runner := &fakeRunner{clock: clk, err: errors.New("exec not found")}
	var stderr strings.Builder

	code := run([]string{"--budget", "3m", "--", "missing"}, runner, clk, mapEnv(nil), io.Discard, &stderr)

	if code != 0 {
		if !strings.Contains(stderr.String(), "exec not found") {
			t.Fatalf("stderr = %q, want command error", stderr.String())
		}
		return
	}
	t.Fatal("run code = 0, want failure")
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
