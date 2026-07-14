package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestContainerStreamReaderEmitsOrderedSnapshotsAndResetsRetries(t *testing.T) {
	dir := t.TempDir()
	var chunks []ports.ContainerStreamChunk
	reader := newContainerStreamReader(dir, func(chunk ports.ContainerStreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})
	stream := strings.Join([]string{
		`{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":1,"delta":"Need "}`,
		`{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":2,"delta":"evidence."}`,
		`{"schema_version":"container_stream.v1","generation_attempt":2,"sequence":0,"delta":"","reset":true}`,
		`{"schema_version":"container_stream.v1","generation_attempt":2,"sequence":1,"delta":"Corrected."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "stream.ndjson"), []byte(stream), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := reader.poll(true); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(chunks) != 4 || chunks[1].Text != "Need evidence." ||
		!chunks[2].Reset || chunks[2].Text != "" || chunks[2].Sequence != 0 ||
		chunks[3].Text != "Corrected." {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestContainerStreamReaderRejectsMalformedOrDiscontinuousRecords(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"unknown field", `{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":1,"delta":"ok","token":"leak"}` + "\n", "unknown field"},
		{"duplicate key", `{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":1,"sequence":2,"delta":"ok"}` + "\n", "duplicate object key"},
		{"sequence gap", `{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":2,"delta":"ok"}` + "\n", "must start"},
		{"reset before preview", `{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":0,"delta":"","reset":true}` + "\n", "not contiguous"},
		{"reset with delta", strings.Join([]string{
			`{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":1,"delta":"old"}`,
			`{"schema_version":"container_stream.v1","generation_attempt":2,"sequence":0,"delta":"leak","reset":true}`,
			"",
		}, "\n"), "empty delta"},
		{"partial final", `{"schema_version":"container_stream.v1","generation_attempt":1`, "end with a newline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "stream.ndjson"), []byte(tt.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			err := newContainerStreamReader(dir, nil).poll(true)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("poll error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestContainerStreamReaderRejectsEscapingSymlinkAndOversizedFile(t *testing.T) {
	t.Run("escaping symlink", func(t *testing.T) {
		dir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.ndjson")
		if err := os.WriteFile(outside, []byte(`{"schema_version":"container_stream.v1"}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "stream.ndjson")); err != nil {
			t.Fatal(err)
		}
		err := newContainerStreamReader(dir, nil).poll(true)
		if err == nil {
			t.Fatal("poll accepted an escaping symlink")
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(
			filepath.Join(dir, "stream.ndjson"),
			[]byte(strings.Repeat("x", ports.MaxContainerStreamFileBytes+1)),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		err := newContainerStreamReader(dir, nil).poll(true)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("poll error = %v, want size rejection", err)
		}
	})
}

func TestContainerStreamReaderRejectsMutationAfterObservation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) error
		want   string
	}{
		{
			name: "rewrite",
			mutate: func(path string) error {
				return os.WriteFile(path, []byte(`{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":1,"delta":"no"}`+"\n"), 0o600)
			},
			want: "changed previously observed bytes",
		},
		{
			name:   "remove",
			mutate: os.Remove,
			want:   "disappeared",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "stream.ndjson")
			initial := `{"schema_version":"container_stream.v1","generation_attempt":1,"sequence":1,"delta":"ok"}` + "\n"
			if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
				t.Fatal(err)
			}
			reader := newContainerStreamReader(dir, nil)
			if err := reader.poll(false); err != nil {
				t.Fatalf("initial poll: %v", err)
			}
			if err := tt.mutate(path); err != nil {
				t.Fatal(err)
			}
			err := reader.poll(false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("poll error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
