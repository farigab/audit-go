// Package main is the audit API server entrypoint.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	accessapp "audit-go/internal/features/access/app"
	accesshttp "audit-go/internal/features/access/http"
	accesspostgres "audit-go/internal/features/access/postgres"
	auditpostgres "audit-go/internal/features/audit/postgres"
	documentsapp "audit-go/internal/features/documents/app"
	documentshttp "audit-go/internal/features/documents/http"
	documentpostgres "audit-go/internal/features/documents/postgres"
	jointventuresapp "audit-go/internal/features/jointventures/app"
	jointventureshttp "audit-go/internal/features/jointventures/http"
	jointventurespostgres "audit-go/internal/features/jointventures/postgres"
	processingpostgres "audit-go/internal/features/processing/postgres"
	processingpython "audit-go/internal/features/processing/python"
	processingworker "audit-go/internal/features/processing/worker"
	regionsapp "audit-go/internal/features/regions/app"
	regionshttp "audit-go/internal/features/regions/http"
	regionspostgres "audit-go/internal/features/regions/postgres"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/httpx"
	"audit-go/internal/platform/httpx/middleware"
	"audit-go/internal/platform/logger"
	platformpostgres "audit-go/internal/platform/postgres"
	"audit-go/internal/platform/security"
	"audit-go/internal/platform/storage"
)

func main() {
	cfg := config.Load()
	cookieCfg := config.LoadCookieConfig()

	log := logger.NewPrettyWithLevel(cfg.LogLevel)

	if cfg.EntraTenantID == "" || cfg.EntraClientID == "" {
		log.Fatal().Msg("ENTRA_TENANT_ID and ENTRA_CLIENT_ID must not be empty")
	}

	db, err := platformpostgres.Connect(cfg.DBurl)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	// repositories
	docRepo := documentpostgres.NewRepository(db)
	auditRepo := auditpostgres.NewRepository(db)
	storageRepo := storage.NewRepository(db)
	processingRepo := processingpostgres.NewRepository(db)
	pythonClient := processingpython.NewClient(cfg.PythonServiceURL)
	authorizer := accesspostgres.NewAuthorizer(db)
	membershipRepo := accesspostgres.NewMembershipRepository(db)
	regionRepo := regionspostgres.NewRepository(db)
	jointVentureRepo := jointventurespostgres.NewRepository(db)
	sessionRepo := accesspostgres.NewSessionRepository(db)
	transactor := platformpostgres.NewTransactor(db)
	blobStore, err := storage.NewAzureBlobStore(storage.AzureBlobConfig{
		AccountName: cfg.AzureStorageAccountName,
		Container:   cfg.AzureStorageContainer,
		Endpoint:    cfg.AzureStorageEndpoint,
	})
	if errors.Is(err, storage.ErrBlobStorageNotConfigured) {
		log.Warn().Msg("Azure Blob Storage is not configured; document upload-url endpoints will return 503")
		blobStore = nil
	} else if err != nil {
		log.Fatal().Err(err).Msg("failed to create Azure Blob Storage client")
	}

	// Microsoft Entra ID token validator
	entra, err := security.NewEntraTokenValidator(security.EntraConfig{
		TenantID: cfg.EntraTenantID,
		ClientID: cfg.EntraClientID,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Entra token validator")
	}

	authService := accessapp.NewService(accessapp.Config{
		TenantID:           cfg.EntraTenantID,
		ClientID:           cfg.EntraClientID,
		ClientSecret:       cfg.EntraClientSecret,
		RedirectURL:        cfg.EntraRedirectURL,
		SuccessRedirectURL: cfg.AuthSuccessRedirectURL,
		AllowedOrigins:     cfg.AllowedOrigins,
		SessionTTL:         cfg.SessionTTL,
		RefreshTTL:         cfg.RefreshTTL,
	}, sessionRepo, entra)

	// use cases
	createDoc := documentsapp.CreateDocumentUseCase{
		DocRepo:        docRepo,
		AuditRepo:      auditRepo,
		StorageRepo:    storageRepo,
		ProcessingRepo: processingRepo,
		Authorizer:     authorizer,
		Transactor:     transactor,
	}

	requestUpload := documentsapp.RequestDocumentUploadUseCase{
		DocRepo:      docRepo,
		AuditRepo:    auditRepo,
		StorageRepo:  storageRepo,
		BlobGateway:  blobStore,
		Authorizer:   authorizer,
		Transactor:   transactor,
		UploadURLTTL: cfg.UploadURLTTL,
	}

	completeUpload := documentsapp.CompleteDocumentUploadUseCase{
		DocRepo:        docRepo,
		AuditRepo:      auditRepo,
		StorageRepo:    storageRepo,
		ProcessingRepo: processingRepo,
		BlobGateway:    blobStore,
		Authorizer:     authorizer,
		Transactor:     transactor,
	}

	deleteDoc := documentsapp.DeleteDocumentUseCase{
		DocRepo:    docRepo,
		AuditRepo:  auditRepo,
		Authorizer: authorizer,
		Transactor: transactor,
	}

	getDoc := documentsapp.GetDocumentUseCase{
		DocRepo:    docRepo,
		Authorizer: authorizer,
	}

	getProcessingStatus := documentsapp.GetDocumentProcessingStatusUseCase{
		DocRepo:        docRepo,
		ProcessingRepo: processingRepo,
		Authorizer:     authorizer,
	}

	listDocumentChunks := documentsapp.ListDocumentChunksUseCase{
		DocRepo:        docRepo,
		ProcessingRepo: processingRepo,
		Authorizer:     authorizer,
	}

	listDocsByJV := documentsapp.ListDocumentsByJVUseCase{
		DocRepo:    docRepo,
		Authorizer: authorizer,
	}

	createMembership := accessapp.CreateMembershipUseCase{
		Repo:       membershipRepo,
		Authorizer: authorizer,
	}
	listMemberships := accessapp.ListMembershipsUseCase{
		Repo:       membershipRepo,
		Authorizer: authorizer,
	}
	deleteMembership := accessapp.DeleteMembershipUseCase{
		Repo:       membershipRepo,
		Authorizer: authorizer,
	}

	createRegion := regionsapp.CreateRegionUseCase{
		Repo:       regionRepo,
		Authorizer: authorizer,
	}
	listRegions := regionsapp.ListRegionsUseCase{Repo: regionRepo}
	getRegion := regionsapp.GetRegionUseCase{
		Repo:       regionRepo,
		Authorizer: authorizer,
	}
	updateRegion := regionsapp.UpdateRegionUseCase{
		Repo:       regionRepo,
		Authorizer: authorizer,
	}
	deleteRegion := regionsapp.DeleteRegionUseCase{
		Repo:       regionRepo,
		Authorizer: authorizer,
	}

	createJV := jointventuresapp.CreateJointVentureUseCase{
		Repo:       jointVentureRepo,
		Authorizer: authorizer,
	}
	listJVsByRegion := jointventuresapp.ListJointVenturesByRegionUseCase{Repo: jointVentureRepo}
	getJV := jointventuresapp.GetJointVentureUseCase{
		Repo:       jointVentureRepo,
		Authorizer: authorizer,
	}
	updateJV := jointventuresapp.UpdateJointVentureUseCase{
		Repo:       jointVentureRepo,
		Authorizer: authorizer,
	}
	deleteJV := jointventuresapp.DeleteJointVentureUseCase{
		Repo:       jointVentureRepo,
		Authorizer: authorizer,
	}

	// router
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := httpx.WriteText(w, http.StatusOK, "ok"); err != nil {
			log.Error().Err(err).Str("path", r.URL.Path).Msg("failed to write response")
		}
	})

	auth := middleware.Auth(entra, authService, accesshttp.SessionCookie)
	accesshttp.RegisterRoutes(
		mux,
		auth,
		accesshttp.NewHandler(log, authService, cookieCfg),
	)
	accesshttp.RegisterMembershipRoutes(
		mux,
		auth,
		accesshttp.NewMembershipHandler(log, createMembership, listMemberships, deleteMembership),
	)
	regionshttp.RegisterRoutes(
		mux,
		auth,
		authorizer,
		regionshttp.NewHandler(log, createRegion, listRegions, getRegion, updateRegion, deleteRegion),
	)
	jointventureshttp.RegisterRoutes(
		mux,
		auth,
		authorizer,
		jointventureshttp.NewHandler(log, createJV, listJVsByRegion, getJV, updateJV, deleteJV),
	)
	documentshttp.RegisterRoutes(
		mux,
		auth,
		documentshttp.NewHandler(
			log,
			createDoc,
			requestUpload,
			completeUpload,
			deleteDoc,
			getDoc,
			getProcessingStatus,
			listDocumentChunks,
			listDocsByJV,
		),
	)

	var app http.Handler = mux
	app = middleware.CSRF(accesshttp.CSRFCookie, accesshttp.CSRFHeader, accesshttp.SessionCookie, accesshttp.RefreshCookie)(app)
	app = middleware.CORSMiddleware(cfg)(app)
	app = middleware.Logging(log)(app)
	app = middleware.RequestContext(app)

	// context cancelled on SIGTERM / SIGINT — shared with the worker
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// worker
	outboxPublisher := processingworker.NewOutboxPublisher(log, processingRepo)
	go outboxPublisher.Start(ctx)

	if blobStore != nil {
		w := processingworker.New(log, processingRepo, blobStore, pythonClient)
		go w.Start(ctx)
	} else {
		log.Warn().Msg("file worker disabled because Azure Blob Storage is not configured")
	}

	srv := &http.Server{
		Addr:              cfg.Port,
		Handler:           app,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.Port).Msg("server started")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-ctx.Done()
	stop()

	log.Info().Msg("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
		os.Exit(1)
	}

	log.Info().Msg("server stopped cleanly")
}
