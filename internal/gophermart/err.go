package gophermart

import (
	"errors"
)

var (
	ErrUserExists         = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrPasswordTooLong    = errors.New("password too long")
	ErrInvalidCredentials = errors.New("invalid credentials")
)
