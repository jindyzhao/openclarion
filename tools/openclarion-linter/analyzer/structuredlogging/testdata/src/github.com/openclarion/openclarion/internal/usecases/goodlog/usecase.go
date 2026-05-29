package goodlog

import (
	"fmt"
	"io"
	"log/slog"
)

func run(w io.Writer, logger *slog.Logger) {
	fmt.Fprintf(w, "response: %s", "ok")
	logger.Info("structured", "status", "ok")
}
