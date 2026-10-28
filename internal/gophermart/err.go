package gophermart

import (
	"errors"
	"fmt"
)

// Доменные ошибки.
var (
	// ErrUserExists - логин уже занят.
	ErrUserExists = errors.New("user already exists")
	// ErrUserNotFound - пользователь не найден.
	ErrUserNotFound = errors.New("user not found")
	// ErrPasswordTooLong - пароль длиннее 72 байт.
	ErrPasswordTooLong = errors.New("password too long")
	// ErrInvalidCredentials - логин или пароль неверные.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrOrderAlreadyCreatedByOther - заказ уже создал другой пользователь.
	ErrOrderAlreadyCreatedByOther = errors.New("order already created by other user")
	// ErrOrderAlreadyCreated - заказ уже создал этот же пользователь.
	ErrOrderAlreadyCreated = errors.New("order already created")
	// ErrOrderNotFound - заказ не найден.
	ErrOrderNotFound = errors.New("order not found")
	// ErrInvalidOrderNumber - номер заказа не проходит проверку по алгоритму Луна.
	ErrInvalidOrderNumber = errors.New("invalid order number")
	// ErrOrderAlreadyProcessed - заказ уже в финальном статусе INVALID или PROCESSED.
	ErrOrderAlreadyProcessed = errors.New("order already processed")
	// ErrInsufficientFunds - на балансе меньше баллов, чем сумма списания.
	ErrInsufficientFunds = errors.New("insufficient funds")
	// ErrAccrualOrderNotFound - система расчета не нашла заказ.
	ErrAccrualOrderNotFound = errors.New("accrual order not found")
	// ErrUnknownOrderStatus - система расчета вернула неизвестный статус.
	ErrUnknownOrderStatus = errors.New("unknown order status")
	// ErrWithdrawalNotPositive - сумма списания меньше или равна нулю.
	ErrWithdrawalNotPositive = errors.New("withdrawal sum must be positive")
)

// AccrualRequestLimitError сообщает, что система расчета ответила 429.
type AccrualRequestLimitError struct {
	// Message - тело ответа системы расчета.
	Message string
	// RetryAfterSeconds - пауза перед следующим запросом, в секундах.
	RetryAfterSeconds int
}

// Error возвращает текст ошибки с паузой в секундах.
func (e *AccrualRequestLimitError) Error() string {
	return fmt.Sprintf("%s, retry after %d seconds", e.Message, e.RetryAfterSeconds)
}

// NewAccrualRequestLimitError создает AccrualRequestLimitError.
func NewAccrualRequestLimitError(message string, retryAfterSeconds int) *AccrualRequestLimitError {
	return &AccrualRequestLimitError{Message: message, RetryAfterSeconds: retryAfterSeconds}
}
