package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/model"
	"go.uber.org/zap"
)

type AuthService interface {
	WithUserID(ctx context.Context, userID model.UserID) context.Context
	UserIDFromContext(ctx context.Context) model.UserID
	BuildToken(userID model.UserID) (string, time.Time, error)
	ParseToken(tokenString string) (*model.Claims, error)
	NeedsRefresh(claims *model.Claims) bool
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

type Handler struct {
	auth   AuthService
	logger *zap.SugaredLogger
}

func New(auth AuthService, logger *zap.SugaredLogger) *Handler {
	handler := &Handler{
		auth:   auth,
		logger: logger,
	}

	return handler
}
