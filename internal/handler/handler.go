package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
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

type MartService interface {
	CreateOrder(ctx context.Context, userID model.UserID, orderNumber string) error
	GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error)
	GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error)
	UserWithdraw(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error
	GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error)
}

type Middleware func(http.HandlerFunc) http.HandlerFunc

type Handler struct {
	auth   AuthService
	mart   MartService
	logger *zap.SugaredLogger
}

func New(auth AuthService, mart MartService, logger *zap.SugaredLogger) *Handler {
	handler := &Handler{
		auth:   auth,
		logger: logger,
		mart:   mart,
	}

	return handler
}

func (h *Handler) APIPostUserRegister(w http.ResponseWriter, r *http.Request) {
	req := model.UserRegisterRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userID, err := h.auth.Register(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, gophermart.ErrInvalidCredentials) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if errors.Is(err, gophermart.ErrUserExists) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if errors.Is(err, gophermart.ErrPasswordTooLong) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = setAuthCookie(h.auth, w, userID)
	if err != nil {
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) APIPostUserLogin(w http.ResponseWriter, r *http.Request) {
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

		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = setAuthCookie(h.auth, w, userID)
	if err != nil {
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) APIPostUserOrder(w http.ResponseWriter, r *http.Request) {
	userID := h.auth.UserIDFromContext(r.Context())
	if userID == 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	orderNumber := strings.TrimSpace(string(raw))
	err = h.mart.CreateOrder(r.Context(), userID, orderNumber)
	if err != nil {
		if errors.Is(err, gophermart.ErrInvalidOrderNumber) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, gophermart.ErrOrderAlreadyCreatedByOther) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if errors.Is(err, gophermart.ErrOrderAlreadyCreated) {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) APIGetUserOrders(w http.ResponseWriter, r *http.Request) {
	userID := h.auth.UserIDFromContext(r.Context())
	if userID == 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	orderList, err := h.mart.GetUserOrderList(r.Context(), userID)
	if err != nil {
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(orderList) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(orderList)
	if err != nil {
		h.logger.Error(err.Error())
	}
}

func (h *Handler) APIGetUserBalance(w http.ResponseWriter, r *http.Request) {
	userID := h.auth.UserIDFromContext(r.Context())
	if userID == 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	balance, err := h.mart.GetUserBalance(r.Context(), userID)
	if err != nil {
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(balance)
	if err != nil {
		h.logger.Error(err.Error())
	}
}

func (h *Handler) APIPostUserWithdraw(w http.ResponseWriter, r *http.Request) {
	userID := h.auth.UserIDFromContext(r.Context())
	if userID == 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	withdraw := model.UserWithdrawRequest{}
	err := json.NewDecoder(r.Body).Decode(&withdraw)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = h.mart.UserWithdraw(r.Context(), userID, withdraw.OrderNumber, withdraw.Sum)
	if err != nil {
		if errors.Is(err, gophermart.ErrInvalidOrderNumber) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, gophermart.ErrWithdrawalNotPositive) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if errors.Is(err, gophermart.ErrInsufficientFunds) {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) APIGetUserWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID := h.auth.UserIDFromContext(r.Context())
	if userID == 0 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	withdrawals, err := h.mart.GetUserWithdrawals(r.Context(), userID)
	if err != nil {
		h.logger.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(withdrawals)
	if err != nil {
		h.logger.Error(err.Error())
	}
}
