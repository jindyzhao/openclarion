package changedfiles

import (
	"strings"
	"testing"
)

func TestSplitNameOnlyOutputAcceptsRepositoryRelativePaths(t *testing.T) {
	files, err := SplitNameOnlyOutput(strings.Join([]string{
		".github/workflows/ci.yml",
		"docs/adr/ADR-0013-per-turn-container-invocation.md",
		"internal/sandbox/runtime.go",
		"",
	}, "\n"))
	if err != nil {
		t.Fatalf("SplitNameOnlyOutput: %v", err)
	}
	want := strings.Join([]string{
		".github/workflows/ci.yml",
		"docs/adr/ADR-0013-per-turn-container-invocation.md",
		"internal/sandbox/runtime.go",
	}, "\n")
	if strings.Join(files, "\n") != want {
		t.Fatalf("files = %#v, want %q", files, want)
	}
}

func TestNormalizeRejectsMalformedPaths(t *testing.T) {
	tests := []string{
		" ./Makefile",
		"./Makefile",
		"docs//README.md",
		"docs/./README.md",
		"docs/../README.md",
		"../README.md",
		"/tmp/README.md",
		"C:/tmp/README.md",
		"https://example.test/README.md",
		`"docs/README.md"`,
		`docs\README.md`,
		"docs/README.md\t",
		"docs/\x00README.md",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if _, err := Normalize(input); err == nil {
				t.Fatalf("Normalize(%q) err = nil, want rejection", input)
			}
		})
	}
}

func TestSplitNameOnlyOutputReportsLineNumber(t *testing.T) {
	_, err := SplitNameOnlyOutput("Makefile\n../outside\n")
	if err == nil {
		t.Fatal("SplitNameOnlyOutput err = nil, want malformed path rejection")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error = %q, want line number", err.Error())
	}
}
