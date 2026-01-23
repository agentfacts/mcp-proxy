package stdio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// DefaultMaxMessageSize is the default maximum size of a single JSON message (1MB).
const DefaultMaxMessageSize = 1024 * 1024

// Reader handles reading newline-delimited JSON messages from stdin.
type Reader struct {
	scanner        *bufio.Scanner
	maxMessageSize int
}

// NewReader creates a new Reader for the given input stream.
func NewReader(in io.Reader) *Reader {
	return NewReaderWithMaxSize(in, DefaultMaxMessageSize)
}

// NewReaderWithMaxSize creates a new Reader with a custom max message size.
func NewReaderWithMaxSize(in io.Reader, maxSize int) *Reader {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSize)

	return &Reader{
		scanner:        scanner,
		maxMessageSize: maxSize,
	}
}

// ReadMessage reads the next JSON message from the input.
// Returns io.EOF when there are no more messages.
func (r *Reader) ReadMessage() ([]byte, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading input: %w", err)
		}
		return nil, io.EOF
	}

	line := r.scanner.Bytes()
	if len(line) == 0 {
		// Skip empty lines
		return r.ReadMessage()
	}

	// Make a copy since scanner reuses the buffer
	msg := make([]byte, len(line))
	copy(msg, line)

	// Validate JSON
	if !json.Valid(msg) {
		return nil, fmt.Errorf("invalid JSON message")
	}

	return msg, nil
}
