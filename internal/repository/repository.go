package repository

import (
	"context"

	"github.com/EzhiG/gophermart-diploma/internal/model"
)

type Storage interface {
	GetUserCredentials(ctx context.Context, login string) (*model.UserCredentials, error)
	CreateUser(ctx context.Context, login, passwordHash string) (model.UserID, error)
	Close() error
}
