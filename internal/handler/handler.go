package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"go.uber.org/zap"
)

type AuthService interface {
	WithUserID(ctx context.Context, userID model.UserID) context.Context
	UserIDFromContext(ctx context.Context) model.UserID
	BuildToken(userID model.UserID) (string, time.Time, error)
	ParseToken(tokenString string) (*model.Claims, error)
	NeedsRefresh(claims *model.Claims) bool
	Register(ctx context.Context, login, password string) (model.UserID, error)
	Login(ctx context.Context, login, password string) (model.UserID, error)
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

func (h *Handler) PostUserRegister(w http.ResponseWriter, r *http.Request) {
	req := model.UserRegisterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userID, err := h.auth.Register(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, gophermart.ErrUserExists) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if errors.Is(err, gophermart.ErrPasswordTooLong) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = setAuthCookie(h.auth, w, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) PostUserLogin(w http.ResponseWriter, r *http.Request) {
	req := model.UserLoginRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userID, err := h.auth.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, gophermart.ErrUserNotFound) || errors.Is(err, gophermart.ErrInvalidCredentials) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = setAuthCookie(h.auth, w, userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
