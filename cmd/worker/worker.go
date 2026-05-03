// Package main is the worker process entrypoint.
package main

import (
	"context"
	"errors"
	"os/signal"
	"syscall"

	processingpostgres "audit-go/internal/features/processing/postgres"
	processingpython "audit-go/internal/features/processing/python"
	processingworker "audit-go/internal/features/processing/worker"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/logger"
	platformpostgres "audit-go/internal/platform/postgres"
	"audit-go/internal/platform/storage"
)

func main() {
	cfg := config.Load()
	log := logger.NewWithLevel(cfg.LogLevel)

	db, err := platformpostgres.Connect(cfg.DBurl)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	processingRepo := processingpostgres.NewRepository(db)
	pythonClient := processingpython.NewClient(cfg.PythonServiceURL)
	blobStore, err := storage.NewAzureBlobStore(storage.AzureBlobConfig{
		AccountName: cfg.AzureStorageAccountName,
		Container:   cfg.AzureStorageContainer,
		Endpoint:    cfg.AzureStorageEndpoint,
	})
	if errors.Is(err, storage.ErrBlobStorageNotConfigured) {
		log.Fatal().Msg("Azure Blob Storage is required by the worker")
	}
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Azure Blob Storage client")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	w := processingworker.New(log, processingRepo, blobStore, pythonClient)
	w.Start(ctx) // blocks until ctx is cancelled
}
