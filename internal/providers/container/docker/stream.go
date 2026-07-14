package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const containerStreamPollInterval = 25 * time.Millisecond

type containerStreamRecord struct {
	SchemaVersion     string `json:"schema_version"`
	GenerationAttempt int    `json:"generation_attempt"`
	Sequence          int    `json:"sequence"`
	Delta             string `json:"delta"`
}

type containerStreamReader struct {
	outputDir         string
	name              string
	onChunk           ports.ContainerStreamHandler
	seenFile          bool
	observed          []byte
	pending           []byte
	generationAttempt int
	sequence          int
	text              string
}

func newContainerStreamReader(outputDir string, onChunk ports.ContainerStreamHandler) *containerStreamReader {
	return &containerStreamReader{
		outputDir: filepath.Clean(outputDir),
		name:      filepath.Base(ports.SandboxStreamPath),
		onChunk:   onChunk,
	}
}

func watchContainerStream(ctx context.Context, reader *containerStreamReader) error {
	ticker := time.NewTicker(containerStreamPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := reader.poll(false); err != nil {
				return err
			}
		case <-ctx.Done():
			return reader.poll(true)
		}
	}
}

func (r *containerStreamReader) poll(final bool) error {
	raw, err := r.readBounded()
	if err != nil {
		if os.IsNotExist(err) {
			if r.seenFile {
				return fmt.Errorf("sandbox stream disappeared during generation")
			}
			return nil
		}
		return fmt.Errorf("read sandbox stream: %w", err)
	}
	r.seenFile = true
	if len(raw) < len(r.observed) {
		return fmt.Errorf("sandbox stream was truncated during generation")
	}
	if !bytes.Equal(raw[:len(r.observed)], r.observed) {
		return fmt.Errorf("sandbox stream changed previously observed bytes")
	}
	if len(raw) > len(r.observed) {
		r.pending = append(r.pending, raw[len(r.observed):]...)
		r.observed = append(r.observed[:0], raw...)
	}

	for {
		newline := bytes.IndexByte(r.pending, '\n')
		if newline < 0 {
			break
		}
		line := append([]byte(nil), bytes.TrimSpace(r.pending[:newline])...)
		r.pending = append(r.pending[:0], r.pending[newline+1:]...)
		if len(line) == 0 {
			continue
		}
		if err := r.accept(line); err != nil {
			return err
		}
	}
	if final && len(bytes.TrimSpace(r.pending)) != 0 {
		return fmt.Errorf("sandbox stream final record must end with a newline")
	}
	return nil
}

func (r *containerStreamReader) readBounded() ([]byte, error) {
	root, err := os.OpenRoot(r.outputDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	before, err := root.Lstat(r.name)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		return nil, fmt.Errorf("sandbox stream must be a direct regular file")
	}
	file, err := root.Open(r.name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return nil, fmt.Errorf("sandbox stream changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(file, ports.MaxContainerStreamFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > ports.MaxContainerStreamFileBytes {
		return nil, fmt.Errorf("sandbox stream exceeds %d bytes", ports.MaxContainerStreamFileBytes)
	}
	return raw, nil
}

func (r *containerStreamReader) accept(line []byte) error {
	var record containerStreamRecord
	if err := strictjson.Unmarshal(line, &record); err != nil {
		return fmt.Errorf("sandbox stream record is invalid: %w", err)
	}
	if record.SchemaVersion != ports.ContainerStreamSchemaVersion {
		return fmt.Errorf("sandbox stream schema_version %q is unsupported", record.SchemaVersion)
	}
	if record.GenerationAttempt <= 0 {
		return fmt.Errorf("sandbox stream generation_attempt must be positive")
	}
	if record.Sequence <= 0 {
		return fmt.Errorf("sandbox stream sequence must be positive")
	}
	if record.Delta == "" {
		return fmt.Errorf("sandbox stream delta must be non-empty")
	}
	if !utf8.ValidString(record.Delta) {
		return fmt.Errorf("sandbox stream delta must be valid UTF-8")
	}
	if len([]byte(record.Delta)) > ports.MaxContainerStreamDeltaBytes {
		return fmt.Errorf("sandbox stream delta exceeds %d bytes", ports.MaxContainerStreamDeltaBytes)
	}

	switch {
	case r.generationAttempt == 0:
		if record.GenerationAttempt != 1 || record.Sequence != 1 {
			return fmt.Errorf("sandbox stream must start at generation_attempt 1 sequence 1")
		}
	case record.GenerationAttempt == r.generationAttempt:
		if record.Sequence != r.sequence+1 {
			return fmt.Errorf("sandbox stream sequence %d is not contiguous after %d", record.Sequence, r.sequence)
		}
	case record.GenerationAttempt == r.generationAttempt+1:
		if record.Sequence != 1 {
			return fmt.Errorf("sandbox stream retry sequence must restart at 1")
		}
		r.text = ""
	default:
		return fmt.Errorf("sandbox stream generation_attempt %d is not contiguous after %d", record.GenerationAttempt, r.generationAttempt)
	}

	text := r.text + record.Delta
	if len([]byte(text)) > ports.MaxContainerStreamTextBytes {
		return fmt.Errorf("sandbox stream accumulated text exceeds %d bytes", ports.MaxContainerStreamTextBytes)
	}
	r.generationAttempt = record.GenerationAttempt
	r.sequence = record.Sequence
	r.text = text
	if r.onChunk != nil {
		if err := r.onChunk(ports.ContainerStreamChunk{
			GenerationAttempt: record.GenerationAttempt,
			Sequence:          record.Sequence,
			Delta:             record.Delta,
			Text:              text,
		}); err != nil {
			return fmt.Errorf("sandbox stream callback: %w", err)
		}
	}
	return nil
}
