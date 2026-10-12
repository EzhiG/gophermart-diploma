package gophermart

import (
	"errors"
	"fmt"
)

var (
	ErrUserExists                 = errors.New("user already exists")
	ErrUserNotFound               = errors.New("user not found")
	ErrPasswordTooLong            = errors.New("password too long")
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrOrderAlreadyCreatedByOther = errors.New("order already created by other user")
	ErrOrderAlreadyCreated        = errors.New("order already created")
	ErrOrderNotFound              = errors.New("order not found")
	ErrInvalidOrderNumber         = errors.New("invalid order number")
	ErrOrderAlreadyProcessed      = errors.New("order already processed")
	ErrInsufficientFunds          = errors.New("insufficient funds")
	ErrAccrualOrderNotFound       = errors.New("accrual order not found")
	ErrUnknownOrderStatus         = errors.New("unknown order status")
	ErrWithdrawalNotPositive      = errors.New("withdrawal sum must be positive")
)

type AccrualRequestLimitError struct {
	Message           string
	RetryAfterSeconds int
}

func (e *AccrualRequestLimitError) Error() string {
	return fmt.Sprintf("%s, retry after %d seconds", e.Message, e.RetryAfterSeconds)
}

func NewAccrualRequestLimitError(message string, retryAfterSeconds int) *AccrualRequestLimitError {
	return &AccrualRequestLimitError{Message: message, RetryAfterSeconds: retryAfterSeconds}
}
