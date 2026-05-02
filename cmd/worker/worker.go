// Package main is the worker process entrypoint.
package main

import (
	"context"
	"os/signal"
	"syscall"

	processingworker "audit-go/internal/features/processing/worker"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/logger"
)

func main() {
	cfg := config.Load()
	log := logger.NewWithLevel(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	w := processingworker.New(log)
	w.Start(ctx) // blocks until ctx is cancelled
}
