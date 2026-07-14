package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type messageProjector struct {
	raw  strings.Builder
	text string
}

func newMessageProjector() *messageProjector { return &messageProjector{} }

func (p *messageProjector) Feed(delta string) (string, bool, error) {
	if !utf8.ValidString(delta) {
		return "", false, fmt.Errorf("LLM stream delta is not valid UTF-8")
	}
	p.raw.WriteString(delta)
	text, found, _, err := extractMessagePrefix(p.raw.String())
	if err != nil {
		return "", false, fmt.Errorf("project diagnosis message preview: %w", err)
	}
	if !found || text == p.text {
		return p.text, false, nil
	}
	if !strings.HasPrefix(text, p.text) {
		return "", false, fmt.Errorf("diagnosis message preview changed non-monotonically")
	}
	p.text = text
	return text, true, nil
}

func extractMessagePrefix(raw string) (string, bool, bool, error) {
	index := skipJSONSpace(raw, 0)
	if index >= len(raw) {
		return "", false, false, nil
	}
	if raw[index] != '{' {
		return "", false, false, fmt.Errorf("structured output must start with an object")
	}
	index++
	for {
		index = skipJSONSpace(raw, index)
		if index >= len(raw) {
			return "", false, false, nil
		}
		if raw[index] == '}' {
			return "", false, true, nil
		}
		key, next, complete, err := decodeJSONStringPrefix(raw, index)
		if err != nil {
			return "", false, false, err
		}
		if !complete {
			return "", false, false, nil
		}
		index = skipJSONSpace(raw, next)
		if index >= len(raw) {
			return "", false, false, nil
		}
		if raw[index] != ':' {
			return "", false, false, fmt.Errorf("object key %q is missing a colon", key)
		}
		index = skipJSONSpace(raw, index+1)
		if key == "message" {
			text, _, valueComplete, err := decodeJSONStringPrefix(raw, index)
			if err != nil {
				return "", false, false, err
			}
			return text, true, valueComplete, nil
		}
		next, complete, err = skipJSONValue(raw, index)
		if err != nil {
			return "", false, false, err
		}
		if !complete {
			return "", false, false, nil
		}
		index = skipJSONSpace(raw, next)
		if index >= len(raw) {
			return "", false, false, nil
		}
		if raw[index] != ',' {
			return "", false, false, fmt.Errorf("object member %q is not followed by a comma", key)
		}
		index++
	}
}

func decodeJSONStringPrefix(raw string, start int) (string, int, bool, error) {
	if start >= len(raw) {
		return "", start, false, nil
	}
	if raw[start] != '"' {
		return "", start, false, fmt.Errorf("expected JSON string")
	}
	index := start + 1
	safeEnd := index
	for index < len(raw) {
		switch raw[index] {
		case '"':
			var decoded string
			if err := json.Unmarshal([]byte(raw[start:index+1]), &decoded); err != nil {
				return "", start, false, err
			}
			return decoded, index + 1, true, nil
		case '\\':
			if index+1 >= len(raw) {
				return decodeJSONPrefix(raw[start+1:safeEnd], start)
			}
			escapeEnd := index + 2
			if raw[index+1] == 'u' {
				escapeEnd = index + 6
				if escapeEnd > len(raw) {
					return decodeJSONPrefix(raw[start+1:safeEnd], start)
				}
				code, ok := decodeHexQuad(raw[index+2 : escapeEnd])
				if !ok {
					return "", start, false, fmt.Errorf("JSON string contains an invalid unicode escape")
				}
				if code >= 0xD800 && code <= 0xDBFF {
					if escapeEnd+6 > len(raw) {
						return decodeJSONPrefix(raw[start+1:safeEnd], start)
					}
					if raw[escapeEnd] != '\\' || raw[escapeEnd+1] != 'u' {
						return "", start, false, fmt.Errorf("JSON string contains an unpaired high surrogate")
					}
					low, ok := decodeHexQuad(raw[escapeEnd+2 : escapeEnd+6])
					if !ok || low < 0xDC00 || low > 0xDFFF {
						return "", start, false, fmt.Errorf("JSON string contains an invalid low surrogate")
					}
					escapeEnd += 6
				}
			}
			index = escapeEnd
			safeEnd = index
		case '\n', '\r':
			return "", start, false, fmt.Errorf("JSON string contains an unescaped newline")
		default:
			_, size := utf8.DecodeRuneInString(raw[index:])
			if size == 0 || (size == 1 && raw[index] >= utf8.RuneSelf) {
				return decodeJSONPrefix(raw[start+1:safeEnd], start)
			}
			index += size
			safeEnd = index
		}
	}
	return decodeJSONPrefix(raw[start+1:safeEnd], start)
}

func decodeHexQuad(raw string) (uint16, bool) {
	if len(raw) != 4 {
		return 0, false
	}
	var value uint16
	for _, char := range raw {
		value <<= 4
		switch {
		case char >= '0' && char <= '9':
			value += uint16(char - '0')
		case char >= 'a' && char <= 'f':
			value += uint16(char-'a') + 10
		case char >= 'A' && char <= 'F':
			value += uint16(char-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func decodeJSONPrefix(content string, start int) (string, int, bool, error) {
	var decoded string
	if err := json.Unmarshal([]byte("\""+content+"\""), &decoded); err != nil {
		return "", start, false, err
	}
	return decoded, start, false, nil
}

func skipJSONValue(raw string, start int) (int, bool, error) {
	decoder := json.NewDecoder(strings.NewReader(raw[start:]))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "unexpected EOF") || strings.Contains(err.Error(), "unexpected end of JSON input") {
			return start, false, nil
		}
		return start, false, err
	}
	return start + int(decoder.InputOffset()), true, nil
}

func skipJSONSpace(raw string, start int) int {
	for start < len(raw) {
		switch raw[start] {
		case ' ', '\t', '\n', '\r':
			start++
		default:
			return start
		}
	}
	return start
}

type previewWriter struct {
	file               *os.File
	buffer             *bufio.Writer
	generation         int
	generationPending  bool
	sequence           int
	previousText       string
	wroteAnyGeneration bool
	writtenBytes       int
}

const sandboxPreviewFileMode = 0o644

func newPreviewWriter(outputDir string) (*previewWriter, error) {
	if err := requireOutputDir(outputDir); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Clean(outputDir))
	if err != nil {
		return nil, fmt.Errorf("open preview output root: %w", err)
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Base(ports.SandboxStreamPath), os.O_CREATE|os.O_EXCL|os.O_WRONLY, sandboxPreviewFileMode)
	if err != nil {
		return nil, fmt.Errorf("create preview stream: %w", err)
	}
	// The host control plane tails this bind-mounted file under a private 0700
	// workspace, and the sandbox runs with a deliberately unrelated UID.
	// #nosec G302 -- cross-UID read access is required inside the private workspace.
	if err := file.Chmod(sandboxPreviewFileMode); err != nil {
		return nil, errors.Join(fmt.Errorf("set preview stream mode: %w", err), file.Close())
	}
	return &previewWriter{file: file, buffer: bufio.NewWriterSize(file, 16*1024)}, nil
}

func (w *previewWriter) BeginGeneration() error {
	w.sequence = 0
	w.previousText = ""
	if !w.wroteAnyGeneration {
		w.generationPending = true
		return nil
	}
	w.generation++
	w.generationPending = false
	return w.writeRecord("", true)
}

func (w *previewWriter) WriteText(text string) error {
	if !strings.HasPrefix(text, w.previousText) {
		return fmt.Errorf("preview text must grow monotonically")
	}
	remaining := strings.TrimPrefix(text, w.previousText)
	if remaining == "" {
		return nil
	}
	if len([]byte(text)) > ports.MaxContainerStreamTextBytes {
		return fmt.Errorf("preview text exceeds %d bytes", ports.MaxContainerStreamTextBytes)
	}
	for remaining != "" {
		delta, rest := splitUTF8Bytes(remaining, ports.MaxContainerStreamDeltaBytes)
		if err := w.writeDelta(delta); err != nil {
			return err
		}
		remaining = rest
	}
	return nil
}

func (w *previewWriter) writeDelta(delta string) error {
	if w.generationPending {
		w.generation++
		w.generationPending = false
		w.wroteAnyGeneration = true
	}
	if !w.wroteAnyGeneration {
		w.generation = 1
		w.wroteAnyGeneration = true
	}
	w.sequence++
	return w.writeRecord(delta, false)
}

func (w *previewWriter) writeRecord(delta string, reset bool) error {
	record := struct {
		SchemaVersion     string `json:"schema_version"`
		GenerationAttempt int    `json:"generation_attempt"`
		Sequence          int    `json:"sequence"`
		Delta             string `json:"delta"`
		Reset             bool   `json:"reset,omitempty"`
	}{
		SchemaVersion:     ports.ContainerStreamSchemaVersion,
		GenerationAttempt: w.generation,
		Sequence:          w.sequence,
		Delta:             delta,
		Reset:             reset,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	recordBytes := len(raw) + 1
	if w.writtenBytes+recordBytes > ports.MaxContainerStreamFileBytes {
		return fmt.Errorf("preview stream exceeds %d bytes", ports.MaxContainerStreamFileBytes)
	}
	if _, err := w.buffer.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write preview stream: %w", err)
	}
	if err := w.buffer.Flush(); err != nil {
		return fmt.Errorf("flush preview stream: %w", err)
	}
	w.writtenBytes += recordBytes
	if !reset {
		w.previousText += delta
	}
	return nil
}

func splitUTF8Bytes(value string, maxBytes int) (string, string) {
	if len([]byte(value)) <= maxBytes {
		return value, ""
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], value[end:]
}

func (w *previewWriter) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	flushErr := w.buffer.Flush()
	closeErr := w.file.Close()
	w.file = nil
	return errors.Join(flushErr, closeErr)
}
