package http

import (
	"github.com/rs/zerolog"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/security"
	"audit-go/internal/usecase"
)

// Dependencies holds all wired dependencies passed to route registration.
type Dependencies struct {
	Log    zerolog.Logger
	Config *config.CookieConfig

	// Entra replaces the previous JWT service + login/logout/refresh use cases.
	Entra *security.EntraTokenValidator

	CreateDocument usecase.CreateDocumentUseCase
	DeleteDocument usecase.DeleteDocumentUseCase
	GetDocument    usecase.GetDocumentUseCase
}
