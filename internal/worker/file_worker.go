// Package worker contains background workers used by the service.
package worker

import "github.com/rs/zerolog"

// FileWorker processes files in the background.
type FileWorker struct {
	Log zerolog.Logger
}

// New creates a new FileWorker with the given logger.
func New(log zerolog.Logger) *FileWorker {
	return &FileWorker{Log: log}
}

// Start blocks and runs the worker loop — call in a goroutine if needed.
func (w *FileWorker) Start() {
	w.Log.Info().Msg("file worker started")
	// aqui vai entrar: poll de fila, processar arquivos, chamar Python etc
}
