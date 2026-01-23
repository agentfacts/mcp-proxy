package stdio

import (
	"io"
	"sync"
)

// Writer handles thread-safe writes to stdout with newline framing.
type Writer struct {
	out io.Writer
	mu  sync.Mutex
}

// NewWriter creates a new Writer for the given output stream.
func NewWriter(out io.Writer) *Writer {
	return &Writer{
		out: out,
	}
}

// Write writes a JSON message followed by a newline.
// It is safe for concurrent use.
func (w *Writer) Write(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write the message
	if _, err := w.out.Write(data); err != nil {
		return err
	}

	// Write newline delimiter
	if _, err := w.out.Write([]byte{'\n'}); err != nil {
		return err
	}

	// Flush if the writer supports it
	if f, ok := w.out.(interface{ Flush() error }); ok {
		return f.Flush()
	}

	return nil
}
