package worker

import "github.com/rs/zerolog"

type FileWorker struct {
	Log zerolog.Logger
}

func New(log zerolog.Logger) *FileWorker {
	return &FileWorker{Log: log}
}

func (w *FileWorker) Start() {
	w.Log.Info().Msg("file worker started")
}
