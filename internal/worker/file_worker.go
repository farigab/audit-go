package worker

import "github.com/rs/zerolog"

type FileWorker struct {
	Log zerolog.Logger
}

func New(log zerolog.Logger) *FileWorker {
	return &FileWorker{Log: log}
}

// Start bloqueia — chame em goroutine ou em main() separado
func (w *FileWorker) Start() {
	w.Log.Info().Msg("file worker started")
	// aqui vai entrar: poll de fila, processar arquivos, chamar Python etc
}
