// Package model содержит общие типы данных системы: идентификаторы, доменные сущности,
// тела HTTP-запросов и ответов, ответ системы расчета.
package model

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// UserID - идентификатор пользователя. 0 означает, что пользователя нет.
type UserID int64

// UserBalance - баланс пользователя. Тело ответа GET /api/user/balance.
type UserBalance struct {
	// Current - доступные баллы.
	Current float64 `json:"current"`
	// Withdrawn - сумма всех списаний.
	Withdrawn float64 `json:"withdrawn"`
}

// UserCredentials - данные пользователя для проверки пароля.
// PasswordHash не попадает в JSON.
type UserCredentials struct {
	ID           UserID `json:"id"`
	Login        string `json:"login"`
	PasswordHash string `json:"-"`
}

// Claims - содержимое JWT-токена авторизации.
// Срок действия токена хранится в RegisteredClaims.ExpiresAt.
type Claims struct {
	jwt.RegisteredClaims
	UserID UserID
}

// UserRegisterRequest - тело запроса POST /api/user/register.
type UserRegisterRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// UserLoginRequest - тело запроса POST /api/user/login.
type UserLoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// UserOrderItem - заказ пользователя. Элемент ответа GET /api/user/orders.
type UserOrderItem struct {
	Number string      `json:"number"`
	Status OrderStatus `json:"status"`
	// Accrual - начисление по заказу. Равно nil, пока начисления нет.
	Accrual *float64 `json:"accrual,omitempty"`
	// UploadedAt - время загрузки заказа. В JSON передается в формате RFC3339.
	UploadedAt time.Time `json:"uploaded_at"`
}

// UserOrderListResponse - тело ответа GET /api/user/orders.
type UserOrderListResponse []UserOrderItem

// UserWithdrawRequest - тело запроса POST /api/user/balance/withdraw.
type UserWithdrawRequest struct {
	// OrderNumber - номер заказа, в счет которого выполняется списание.
	OrderNumber string `json:"order"`
	// Sum - сумма списания в баллах.
	Sum float64 `json:"sum"`
}

// UserWithdrawalItem - списание пользователя. Элемент UserWithdrawalListResponse.
type UserWithdrawalItem struct {
	OrderNumber string  `json:"order"`
	Sum         float64 `json:"sum"`
	// ProcessedAt - время списания. В JSON передается в формате RFC3339.
	ProcessedAt time.Time `json:"processed_at"`
}

// UserWithdrawalListResponse - тело ответа GET /api/user/withdrawals.
type UserWithdrawalListResponse []UserWithdrawalItem

// AccrualOrder - ответ системы расчета на GET /api/orders/{number}.
type AccrualOrder struct {
	Order  string             `json:"order"`
	Status AccrualOrderStatus `json:"status"`
	// Accrual - начисление по заказу. Равно nil, если начисления нет.
	Accrual *float64 `json:"accrual,omitempty"`
}

// OrderStatus - статус заказа в системе.
type OrderStatus string

// Статусы заказа. INVALID и PROCESSED - финальные: заказ в этих статусах не меняется.
const (
	// OrderStatusNew - заказ загружен в систему, но не попал в обработку.
	OrderStatusNew OrderStatus = "NEW"
	// OrderStatusProcessing - вознаграждение за заказ рассчитывается.
	OrderStatusProcessing OrderStatus = "PROCESSING"
	// OrderStatusInvalid - система расчёта вознаграждений отказала в расчёте.
	OrderStatusInvalid OrderStatus = "INVALID"
	// OrderStatusProcessed - данные по заказу проверены и информация о расчёте успешно получена.
	OrderStatusProcessed OrderStatus = "PROCESSED"
)

// AccrualOrderStatus - статус заказа в системе расчета.
type AccrualOrderStatus string

// Статусы заказа в системе расчета. INVALID и PROCESSED - финальные.
const (
	// AccrualOrderStatusRegistered - заказ зарегистрирован, но начисление не рассчитано.
	AccrualOrderStatusRegistered AccrualOrderStatus = "REGISTERED"
	// AccrualOrderStatusInvalid - заказ не принят к расчёту, и вознаграждение не будет начислено.
	AccrualOrderStatusInvalid AccrualOrderStatus = "INVALID"
	// AccrualOrderStatusProcessing - расчёт начисления в процессе.
	AccrualOrderStatusProcessing AccrualOrderStatus = "PROCESSING"
	// AccrualOrderStatusProcessed - расчёт начисления окончен.
	AccrualOrderStatusProcessed AccrualOrderStatus = "PROCESSED"
)
