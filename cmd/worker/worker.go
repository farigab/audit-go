// Package main is the worker process entrypoint.
package main

import (
	"context"
	"os/signal"
	"syscall"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/logger"
	"audit-go/internal/worker"
)

func main() {
	cfg := config.Load()
	log := logger.NewWithLevel(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	w := worker.New(log)
	w.Start(ctx) // blocks until ctx is cancelled
}
