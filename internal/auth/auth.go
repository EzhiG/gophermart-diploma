package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

type UserStorage interface {
	GetUserCredentials(ctx context.Context, login string) (*model.UserCredentials, error)
	CreateUser(ctx context.Context, login, passwordHash string) (model.UserID, error)
}

type Auth struct {
	secretKey string
	tokenExp  time.Duration
	storage   UserStorage
}
type ctxKey string

const userIDKey ctxKey = "user_id"

func New(secretKey string, tokenExp time.Duration, storage UserStorage) *Auth {
	return &Auth{secretKey, tokenExp, storage}
}

func (a *Auth) WithUserID(ctx context.Context, userID model.UserID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func (a *Auth) UserIDFromContext(ctx context.Context) model.UserID {
	userID, ok := ctx.Value(userIDKey).(model.UserID)
	if !ok {
		return 0
	}

	return userID
}

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

func (a *Auth) Register(ctx context.Context, login, password string) (model.UserID, error) {
	if len(login) == 0 || len(password) == 0 {
		return 0, gophermart.ErrInvalidCredentials
	}
	hash, err := a.hashPassword(password)
	if err != nil {
		return 0, err
	}

	return a.storage.CreateUser(ctx, login, hash)
}

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
