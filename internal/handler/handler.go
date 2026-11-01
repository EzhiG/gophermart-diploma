// Package handler - HTTP-слой системы: хендлеры эндпоинтов /api/user/* и middleware.
// Сервисы подключаются через интерфейсы AuthService и MartService.
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

// AuthService - сервис авторизации, который использует Handler.
// Контракт методов описан в auth.Auth.
type AuthService interface {
	WithUserID(ctx context.Context, userID model.UserID) context.Context
	UserIDFromContext(ctx context.Context) model.UserID
	BuildToken(userID model.UserID) (string, time.Time, error)
	ParseToken(tokenString string) (*model.Claims, error)
	NeedsRefresh(claims *model.Claims) bool
	Register(ctx context.Context, login, password string) (model.UserID, error)
	Login(ctx context.Context, login, password string) (model.UserID, error)
}

// MartService - сервис накопительной системы, который использует Handler.
// Контракт методов описан в gophermart.Gophermart.
type MartService interface {
	CreateOrder(ctx context.Context, userID model.UserID, orderNumber string) error
	GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error)
	GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error)
	UserWithdraw(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error
	GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error)
}

// Middleware - обертка над http.HandlerFunc.
type Middleware func(http.HandlerFunc) http.HandlerFunc

// Handler содержит хендлеры и middleware системы.
type Handler struct {
	auth   AuthService
	mart   MartService
	logger *zap.SugaredLogger
}

// New создает Handler.
func New(auth AuthService, mart MartService, logger *zap.SugaredLogger) *Handler {
	handler := &Handler{
		auth:   auth,
		logger: logger,
		mart:   mart,
	}

	return handler
}

// APIPostUserRegister обрабатывает POST /api/user/register.
// Тело запроса - model.UserRegisterRequest в JSON.
// При успехе устанавливает cookie авторизации: пользователь сразу авторизован.
// Коды ответа:
//   - 200 - пользователь создан;
//   - 400 - некорректный JSON, пустые логин/пароль, логин длиннее 255 символов или пароль длиннее 72 байт;
//   - 409 - логин занят;
//   - 500 - внутренняя ошибка.
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
			w.WriteHeader(http.StatusBadRequest)
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

// APIPostUserLogin обрабатывает POST /api/user/login.
// Тело запроса - model.UserLoginRequest в JSON.
// При успехе устанавливает cookie авторизации.
// Коды ответа:
//   - 200 - пользователь авторизован;
//   - 400 - тело запроса не JSON;
//   - 401 - логин или пароль неверные;
//   - 500 - внутренняя ошибка.
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

// APIPostUserOrder обрабатывает POST /api/user/orders.
// Тело запроса - номер заказа в text/plain. Пробелы по краям игнорируются.
// Коды ответа:
//   - 202 - заказ принят;
//   - 200 - этот пользователь уже загрузил заказ;
//   - 400 - тело запроса пустое;
//   - 401 - пользователь не авторизован;
//   - 409 - заказ загрузил другой пользователь;
//   - 422 - номер не проходит проверку по алгоритму Луна;
//   - 500 - внутренняя ошибка.
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
	if orderNumber == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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

// APIGetUserOrders обрабатывает GET /api/user/orders.
// Тело ответа - model.UserOrderListResponse в JSON.
// Коды ответа:
//   - 200 - список заказов;
//   - 204 - заказов нет;
//   - 401 - пользователь не авторизован;
//   - 500 - внутренняя ошибка.
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

// APIGetUserBalance обрабатывает GET /api/user/balance.
// Тело ответа - model.UserBalance в JSON.
// Коды ответа:
//   - 200 - баланс;
//   - 401 - пользователь не авторизован;
//   - 500 - внутренняя ошибка.
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

// APIPostUserWithdraw обрабатывает POST /api/user/balance/withdraw.
// Тело запроса - model.UserWithdrawRequest в JSON.
// Коды ответа:
//   - 200 - баллы списаны;
//   - 400 - тело запроса не JSON, или сумма меньше или равна нулю;
//   - 401 - пользователь не авторизован;
//   - 402 - на балансе недостаточно баллов;
//   - 422 - номер заказа не проходит проверку по алгоритму Луна;
//   - 500 - внутренняя ошибка.
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

// APIGetUserWithdrawals обрабатывает GET /api/user/withdrawals.
// Тело ответа - model.UserWithdrawalListResponse в JSON.
// Коды ответа:
//   - 200 - список списаний;
//   - 204 - списаний нет;
//   - 401 - пользователь не авторизован;
//   - 500 - внутренняя ошибка.
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
