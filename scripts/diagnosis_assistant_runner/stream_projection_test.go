package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestMessageProjectorHandlesEscapesAndSurrogateChunkBoundaries(t *testing.T) {
	projector := newMessageProjector()
	text, changed, err := projector.Feed(`{"schema_version":"diagnosis_turn.v1","message":"Line\nemoji \uD83D`)
	if err != nil {
		t.Fatalf("first Feed: %v", err)
	}
	if !changed || text != "Line\nemoji " {
		t.Fatalf("first projection = %q changed=%t", text, changed)
	}
	text, changed, err = projector.Feed(`\uDE00 done","confidence":"high"}`)
	if err != nil {
		t.Fatalf("second Feed: %v", err)
	}
	if !changed || text != "Line\nemoji 😀 done" {
		t.Fatalf("second projection = %q changed=%t", text, changed)
	}
}

func TestPreviewWriterUsesContiguousVisibleGenerationsAndBoundedRecords(t *testing.T) {
	dir := t.TempDir()
	writer, err := newPreviewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	writer.BeginGeneration()
	writer.BeginGeneration()
	if err := writer.WriteText("Corrected"); err != nil {
		t.Fatal(err)
	}
	writer.BeginGeneration()
	if err := writer.WriteText("Final"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	streamPath := filepath.Join(dir, filepath.Base(ports.SandboxStreamPath))
	info, err := os.Stat(streamPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != sandboxPreviewFileMode {
		t.Fatalf("stream mode = %04o, want %04o", got, sandboxPreviewFileMode)
	}
	raw, err := os.ReadFile(streamPath) // #nosec G304 -- dir is created by t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("stream = %s", raw)
	}
	for index, line := range lines {
		var record struct {
			GenerationAttempt int `json:"generation_attempt"`
			Sequence          int `json:"sequence"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.GenerationAttempt != index+1 || record.Sequence != 1 {
			t.Fatalf("record[%d] = %+v", index, record)
		}
	}
}

func TestPreviewWriterSplitsUTF8WithoutExceedingDeltaLimit(t *testing.T) {
	dir := t.TempDir()
	writer, err := newPreviewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	writer.BeginGeneration()
	text := strings.Repeat("界", ports.MaxContainerStreamDeltaBytes/3+1)
	if err := writer.WriteText(text); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, filepath.Base(ports.SandboxStreamPath))) // #nosec G304 -- dir is created by t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var rebuilt strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var record struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if len([]byte(record.Delta)) > ports.MaxContainerStreamDeltaBytes {
			t.Fatalf("delta bytes = %d", len([]byte(record.Delta)))
		}
		rebuilt.WriteString(record.Delta)
	}
	if rebuilt.String() != text {
		t.Fatalf("rebuilt preview differs")
	}
}
