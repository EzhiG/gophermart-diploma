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

type Claims struct {
	jwt.RegisteredClaims
	UserID model.UserID
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

func (auth *Auth) WithUserID(ctx context.Context, userID model.UserID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func (auth *Auth) UserIDFromContext(ctx context.Context) model.UserID {
	userID, ok := ctx.Value(userIDKey).(model.UserID)
	if !ok {
		return 0
	}

	return userID
}

func (auth *Auth) IsTokenExpiredOnlyError(err error) bool {
	var validationErr *jwt.ValidationError
	return errors.As(err, &validationErr) && validationErr.Errors == jwt.ValidationErrorExpired
}

func (auth *Auth) BuildToken(userID model.UserID) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(auth.tokenExp)),
		},
		UserID: userID,
	})

	tokenString, err := token.SignedString([]byte(auth.secretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (auth *Auth) ParseToken(tokenString string) (model.UserID, error) {
	claims := &Claims{}
	keyFunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(auth.secretKey), nil
	}
	_, err := jwt.ParseWithClaims(tokenString, claims, keyFunc)

	return claims.UserID, err
}

func (auth *Auth) hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return "", gophermart.ErrPasswordTooLong
		}
		return "", err
	}
	return string(bytes), nil
}

func (auth *Auth) checkPasswordHash(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return gophermart.ErrInvalidCredentials
	}

	return nil
}

func (auth *Auth) Register(ctx context.Context, login, password string) (model.UserID, error) {
	hash, err := auth.hashPassword(password)
	if err != nil {
		return 0, err
	}

	return auth.storage.CreateUser(ctx, login, hash)
}

func (auth *Auth) Login(ctx context.Context, login, password string) (model.UserID, error) {
	credentials, err := auth.storage.GetUserCredentials(ctx, login)
	if err != nil {
		if errors.Is(err, gophermart.ErrUserNotFound) {
			return 0, gophermart.ErrInvalidCredentials
		}
		return 0, err
	}
	err = auth.checkPasswordHash(password, credentials.PasswordHash)
	if err != nil {
		return 0, err
	}

	return credentials.ID, nil
}
