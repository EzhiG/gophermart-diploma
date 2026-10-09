package repository

import (
	"context"

	"github.com/EzhiG/gophermart-diploma/internal/model"
)

type Storage interface {
	CreateUser(ctx context.Context, login, passwordHash string) (model.UserID, error)
	GetUserCredentials(ctx context.Context, login string) (*model.UserCredentials, error)
	GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error)
	CreateUserOrder(ctx context.Context, userID model.UserID, orderNumber string) (bool, error)
	GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error)
	UpdateUserOrderStatus(ctx context.Context, orderNumber string, status model.OrderStatus) error
	SetUserOrderProcessed(ctx context.Context, orderNumber string, accrual float64) error
	GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error)
	WithdrawUserBalance(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error
	GetUnfinishedOrders(ctx context.Context) ([]string, error)
	Close() error
}
