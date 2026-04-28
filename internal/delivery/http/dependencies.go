package http

import (
	"github.com/rs/zerolog"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/security"
	"audit-go/internal/repository"
	"audit-go/internal/usecase"
)

// Dependencies holds all wired dependencies passed to route registration.
type Dependencies struct {
	Log         zerolog.Logger
	Config      *config.CookieConfig
	JWT         security.TokenService
	UserRepo    repository.UserRepository
	RefreshRepo repository.RefreshTokenRepository

	Login  usecase.LoginUseCase
	Logout usecase.LogoutUseCase

	CreateDocument usecase.CreateDocumentUseCase
	DeleteDocument usecase.DeleteDocumentUseCase
	GetDocument    usecase.GetDocumentUseCase
}
