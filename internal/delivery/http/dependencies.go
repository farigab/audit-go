package http

import (
	"github.com/rs/zerolog"

	"audit-go/internal/platform/config"
	"audit-go/internal/repository"
	"audit-go/internal/platform/security"
	"audit-go/internal/usecase"
)

type Dependencies struct {
	Log         zerolog.Logger
	Config      *config.CookieConfig
	JWT         security.TokenService
	UserRepo    repository.UserRepository
	RefreshRepo repository.RefreshTokenRepository

	CreateDocument usecase.CreateDocumentUseCase
	DeleteDocument usecase.DeleteDocumentUseCase
	GetDocument    usecase.GetDocumentUseCase
}
