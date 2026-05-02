// Package worker contains background workers used by processing.
package worker

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

const pollInterval = 30 * time.Second

// FileWorker polls for unprocessed documents and sends them to the Python service.
type FileWorker struct {
	Log zerolog.Logger
}

// New creates a new FileWorker with the given logger.
func New(log zerolog.Logger) *FileWorker {
	return &FileWorker{Log: log}
}

// Start runs the worker loop until ctx is cancelled.
func (w *FileWorker) Start(ctx context.Context) {
	w.Log.Info().Msg("file worker started")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.Log.Info().Msg("file worker stopped")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *FileWorker) poll(ctx context.Context) {
	w.Log.Debug().Msg("worker polling for unprocessed documents")
	// TODO: fetch unprocessed docs, call Python /parse, store chunks + embeddings.
}
