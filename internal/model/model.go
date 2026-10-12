package model

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
)

type UserID int64

type UserBalance struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type UserCredentials struct {
	ID           UserID `json:"id"`
	Login        string `json:"login"`
	PasswordHash string `json:"-"`
}

type Claims struct {
	jwt.RegisteredClaims
	UserID UserID
}

type UserRegisterRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type UserLoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type UserOrderItem struct {
	Number     string      `json:"number"`
	Status     OrderStatus `json:"status"`
	Accrual    *float64    `json:"accrual,omitempty"`
	UploadedAt time.Time   `json:"uploaded_at"`
}

type UserOrderListResponse []UserOrderItem

type UserWithdrawRequest struct {
	OrderNumber string  `json:"order"`
	Sum         float64 `json:"sum"`
}

type UserWithdrawalItem struct {
	OrderNumber string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type UserWithdrawalListResponse []UserWithdrawalItem

type AccrualOrder struct {
	Order   string             `json:"order"`
	Status  AccrualOrderStatus `json:"status"`
	Accrual *float64           `json:"accrual,omitempty"`
}

type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "NEW"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusInvalid    OrderStatus = "INVALID"
	OrderStatusProcessed  OrderStatus = "PROCESSED"
)

type AccrualOrderStatus string

const (
	AccrualOrderStatusRegistered AccrualOrderStatus = "REGISTERED"
	AccrualOrderStatusProcessing AccrualOrderStatus = "PROCESSING"
	AccrualOrderStatusInvalid    AccrualOrderStatus = "INVALID"
	AccrualOrderStatusProcessed  AccrualOrderStatus = "PROCESSED"
)
