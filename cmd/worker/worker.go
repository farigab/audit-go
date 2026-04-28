// Package main is the worker process entrypoint.
package main

import (
	"os"
	"os/signal"
	"syscall"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/logger"
	"audit-go/internal/worker"
)

func main() {
	cfg := config.Load()
	log := logger.NewWithLevel(cfg.LogLevel)

	w := worker.New(log)

	// roda em goroutine para não bloquear o signal handler
	go w.Start()

	// aguarda SIGTERM ou SIGINT para desligar graciosamente
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	log.Info().Msg("worker shutting down")
}
