package audit

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Writer provides async buffered writing of audit records.
type Writer struct {
	store *Store

	// Buffer
	buffer    []*Record
	bufferMu  sync.Mutex
	bufferMax int

	// Flush settings
	flushInterval time.Duration
	flushChan     chan struct{}

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	written  int64
	dropped  int64
	flushes  int64
	metricMu sync.Mutex
}

// WriterConfig holds configuration for the audit writer.
type WriterConfig struct {
	BufferSize    int           // Max records to buffer before flush
	FlushInterval time.Duration // How often to flush
}

// NewWriter creates a new async audit writer.
func NewWriter(store *Store, cfg WriterConfig) *Writer {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 100
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Writer{
		store:         store,
		buffer:        make([]*Record, 0, cfg.BufferSize),
		bufferMax:     cfg.BufferSize,
		flushInterval: cfg.FlushInterval,
		flushChan:     make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}

	return w
}

// Start begins the background flush loop.
func (w *Writer) Start() {
	w.wg.Add(1)
	go w.flushLoop()
	log.Info().
		Int("buffer_size", w.bufferMax).
		Dur("flush_interval", w.flushInterval).
		Msg("Audit writer started")
}

// Write adds a record to the buffer.
func (w *Writer) Write(record *Record) {
	w.bufferMu.Lock()
	defer w.bufferMu.Unlock()

	// Check if buffer is full
	if len(w.buffer) >= w.bufferMax {
		// Trigger async flush
		select {
		case w.flushChan <- struct{}{}:
		default:
		}

		// Drop oldest if still full
		if len(w.buffer) >= w.bufferMax {
			w.buffer = w.buffer[1:]
			w.metricMu.Lock()
			w.dropped++
			w.metricMu.Unlock()
		}
	}

	w.buffer = append(w.buffer, record)
}

// flushLoop periodically flushes the buffer.
func (w *Writer) flushLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			// Final flush on shutdown
			w.flush()
			return

		case <-ticker.C:
			w.flush()

		case <-w.flushChan:
			w.flush()
		}
	}
}

// flush writes buffered records to the store.
func (w *Writer) flush() {
	w.bufferMu.Lock()
	if len(w.buffer) == 0 {
		w.bufferMu.Unlock()
		return
	}

	// Swap buffer
	records := w.buffer
	w.buffer = make([]*Record, 0, w.bufferMax)
	w.bufferMu.Unlock()

	// Write to store
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := w.store.InsertBatch(ctx, records); err != nil {
		log.Error().Err(err).Int("count", len(records)).Msg("Failed to flush audit records")
		// Records are lost - could implement retry queue here
		w.metricMu.Lock()
		w.dropped += int64(len(records))
		w.metricMu.Unlock()
		return
	}

	w.metricMu.Lock()
	w.written += int64(len(records))
	w.flushes++
	w.metricMu.Unlock()

	log.Debug().Int("count", len(records)).Msg("Flushed audit records")
}

// Flush forces an immediate flush of the buffer.
func (w *Writer) Flush() {
	w.flush()
}

// Stop stops the writer and flushes remaining records.
func (w *Writer) Stop() {
	log.Info().Msg("Stopping audit writer...")
	w.cancel()
	w.wg.Wait()

	// Get final stats
	stats := w.Stats()
	log.Info().
		Int64("written", stats.Written).
		Int64("dropped", stats.Dropped).
		Int64("flushes", stats.Flushes).
		Msg("Audit writer stopped")
}

// WriterStats contains writer statistics.
type WriterStats struct {
	Written    int64
	Dropped    int64
	Flushes    int64
	BufferSize int
}

// Stats returns current writer statistics.
func (w *Writer) Stats() WriterStats {
	w.metricMu.Lock()
	defer w.metricMu.Unlock()

	w.bufferMu.Lock()
	bufferSize := len(w.buffer)
	w.bufferMu.Unlock()

	return WriterStats{
		Written:    w.written,
		Dropped:    w.dropped,
		Flushes:    w.flushes,
		BufferSize: bufferSize,
	}
}
