// Package auth отвечает за пользователей и авторизацию: регистрацию, вход,
// выпуск и проверку JWT-токенов, передачу UserID через context.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

// UserStorage - хранилище пользователей, которое использует Auth.
// Контракт методов описан в repository.PGStorage.
type UserStorage interface {
	GetUserCredentials(ctx context.Context, login string) (*model.UserCredentials, error)
	CreateUser(ctx context.Context, login, passwordHash string) (model.UserID, error)
}

// Auth - сервис авторизации.
type Auth struct {
	secretKey string
	tokenExp  time.Duration
	storage   UserStorage
}
type ctxKey string

const userIDKey ctxKey = "user_id"
const loginMaxChars = 255

// New создает сервис.
// secretKey - ключ подписи токенов.
// tokenExp - срок действия токена.
func New(secretKey string, tokenExp time.Duration, storage UserStorage) *Auth {
	return &Auth{secretKey, tokenExp, storage}
}

// WithUserID возвращает копию ctx с userID.
func (a *Auth) WithUserID(ctx context.Context, userID model.UserID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext возвращает UserID из ctx.
// Если UserID в ctx нет, возвращает 0.
func (a *Auth) UserIDFromContext(ctx context.Context) model.UserID {
	userID, ok := ctx.Value(userIDKey).(model.UserID)
	if !ok {
		return 0
	}

	return userID
}

// BuildToken выпускает токен для userID.
// Возвращает подписанный токен и время окончания его действия
func (a *Auth) BuildToken(userID model.UserID) (string, time.Time, error) {
	expiresAt := time.Now().Add(a.tokenExp)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		UserID: userID,
	})

	tokenString, err := token.SignedString([]byte(a.secretKey))
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// ParseToken проверяет подпись и срок действия токена и возвращает его содержимое.
// Если токен пустой, поврежден, подписан другим ключом или истек, возвращает ошибку.
func (a *Auth) ParseToken(tokenString string) (*model.Claims, error) {
	claims := &model.Claims{}
	keyFunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(a.secretKey), nil
	}
	_, err := jwt.ParseWithClaims(tokenString, claims, keyFunc)

	if err != nil {
		return nil, err
	}

	return claims, nil
}

// NeedsRefresh сообщает, что токен нужно перевыпустить.
// Возвращает true, если до окончания действия осталось меньше половины tokenExp или срок действия не задан.
func (a *Auth) NeedsRefresh(claims *model.Claims) bool {
	if claims.ExpiresAt == nil {
		return true
	}

	return time.Until(claims.ExpiresAt.Time) < a.tokenExp/2
}

func (a *Auth) hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", gophermart.ErrPasswordTooLong
		}
		return "", err
	}
	return string(bytes), nil
}

func (a *Auth) checkPasswordHash(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return gophermart.ErrInvalidCredentials
		}
		return err
	}

	return nil
}

// Register создает пользователя и возвращает его UserID.
// Пароль хранится в виде криптографического хеша.
// Возвращает:
//   - gophermart.ErrInvalidCredentials, если логин или пароль пустые либо логин длиннее 255 символов;
//   - gophermart.ErrPasswordTooLong, если пароль длиннее 72 байт;
//   - gophermart.ErrUserExists, если логин занят;
//   - ошибку хранилища без изменений.
func (a *Auth) Register(ctx context.Context, login, password string) (model.UserID, error) {
	if len(login) == 0 || len(password) == 0 {
		return 0, gophermart.ErrInvalidCredentials
	}

	if utf8.RuneCountInString(login) > loginMaxChars {
		return 0, gophermart.ErrInvalidCredentials
	}

	hash, err := a.hashPassword(password)
	if err != nil {
		return 0, err
	}

	return a.storage.CreateUser(ctx, login, hash)
}

// Login проверяет логин и пароль и возвращает UserID.
// Возвращает gophermart.ErrInvalidCredentials, если логин или пароль пустые,
// пользователь не найден или пароль неверный. Остальные ошибки возвращает без изменений.
func (a *Auth) Login(ctx context.Context, login, password string) (model.UserID, error) {
	if len(login) == 0 || len(password) == 0 {
		return 0, gophermart.ErrInvalidCredentials
	}
	credentials, err := a.storage.GetUserCredentials(ctx, login)
	if err != nil {
		if errors.Is(err, gophermart.ErrUserNotFound) {
			return 0, gophermart.ErrInvalidCredentials
		}
		return 0, err
	}
	err = a.checkPasswordHash(password, credentials.PasswordHash)
	if err != nil {
		return 0, err
	}

	return credentials.ID, nil
}
