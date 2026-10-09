package gophermart

import (
	"errors"
)

var (
	ErrUserExists                 = errors.New("user already exists")
	ErrUserNotFound               = errors.New("user not found")
	ErrPasswordTooLong            = errors.New("password too long")
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrOrderAlreadyCreatedByOther = errors.New("order already created by other user")
	ErrOrderNotFound              = errors.New("order not found")
	ErrOrderAlreadyProcessed      = errors.New("order already processed")
	ErrInsufficientFunds          = errors.New("insufficient funds")
)
