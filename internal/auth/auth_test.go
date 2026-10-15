package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuth_BuildParseToken(t *testing.T) {
	now := time.Now()
	a := New("secret", time.Hour, nil)
	token, expires, err := a.BuildToken(42)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	t.Run("valid token", func(t *testing.T) {
		assert.WithinDuration(t, now.Add(time.Hour), expires, time.Second)

		claims, err := a.ParseToken(token)
		require.NoError(t, err)
		require.NotNil(t, claims.ExpiresAt)
		require.WithinDuration(t, expires, claims.ExpiresAt.Time, time.Second)
		assert.Equal(t, model.UserID(42), claims.UserID)
	})

	t.Run("wrong secret", func(t *testing.T) {
		other := New("other", time.Hour, nil)
		_, err := other.ParseToken(token)
		assert.ErrorIs(t, err, jwt.ErrSignatureInvalid)
	})

	t.Run("expired token", func(t *testing.T) {
		a := New("secret", -time.Minute, nil)
		token, _, err := a.BuildToken(42)
		require.NoError(t, err)

		claims, err := a.ParseToken(token)
		require.Nil(t, claims)
		require.ErrorIs(t, err, jwt.ErrTokenExpired)
	})
}

func TestAuth_NeedsRefresh(t *testing.T) {
	a := New("secret", time.Hour, nil)
	tests := []struct {
		name      string
		expiresAt *jwt.NumericDate
		want      bool
	}{
		{
			name:      "no expire time",
			expiresAt: nil,
			want:      true,
		},
		{
			name:      "50 min left, token is ok",
			expiresAt: jwt.NewNumericDate(time.Now().Add(50 * time.Minute)),
			want:      false,
		},
		{
			name:      "10 min left, needs refresh",
			expiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			want:      true,
		},
		{
			name:      "90 min passed, needs refresh",
			expiresAt: jwt.NewNumericDate(time.Now().Add(-90 * time.Minute)),
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &model.Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: tt.expiresAt,
				},
			}
			assert.Equal(t, tt.want, a.NeedsRefresh(claims))
		})
	}
}

type fakeStorage struct {
	creds    *model.UserCredentials
	id       model.UserID
	err      error
	gotLogin string
	gotHash  string
}

func (s *fakeStorage) GetUserCredentials(_ context.Context, login string) (*model.UserCredentials, error) {
	s.gotLogin = login
	return s.creds, s.err
}

func (s *fakeStorage) CreateUser(_ context.Context, login, passwordHash string) (model.UserID, error) {
	s.gotLogin = login
	s.gotHash = passwordHash
	return s.id, s.err
}

var _ UserStorage = (*fakeStorage)(nil)

func TestAuth_Register(t *testing.T) {
	tests := []struct {
		name     string
		login    string
		password string
		storage  *fakeStorage
		wantID   model.UserID
		wantErr  error
	}{
		{
			name:     "empty login",
			login:    "",
			password: "pass",
			storage:  &fakeStorage{},
			wantID:   0,
			wantErr:  gophermart.ErrInvalidCredentials,
		},
		{
			name:     "empty password",
			login:    "login",
			password: "",
			storage:  &fakeStorage{},
			wantID:   0,
			wantErr:  gophermart.ErrInvalidCredentials,
		},
		{
			name:     "too long password",
			login:    "login",
			password: strings.Repeat("a", 73),
			storage:  &fakeStorage{},
			wantID:   0,
			wantErr:  gophermart.ErrPasswordTooLong,
		},
		{
			name:     "user exists",
			login:    "login",
			password: "pass",
			storage:  &fakeStorage{err: gophermart.ErrUserExists},
			wantID:   0,
			wantErr:  gophermart.ErrUserExists,
		},
		{
			name:     "user created",
			login:    "login",
			password: "pass",
			storage:  &fakeStorage{id: 7},
			wantID:   7,
			wantErr:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New("secret", time.Hour, tt.storage)
			userID, err := a.Register(t.Context(), tt.login, tt.password)
			assert.Equal(t, tt.wantID, userID)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.login, tt.storage.gotLogin)
			assert.NotEqual(t, tt.password, tt.storage.gotHash)
			assert.NoError(t, a.checkPasswordHash(tt.password, tt.storage.gotHash))
		})
	}
}

func TestAuth_Login(t *testing.T) {
	a := New("secret", time.Hour, nil)
	hash, err := a.hashPassword("pass")
	require.NoError(t, err)
	dbError := errors.New("DB down")
	tests := []struct {
		name     string
		login    string
		password string
		storage  *fakeStorage
		wantID   model.UserID
		wantErr  error
	}{
		{
			name:     "empty login",
			login:    "",
			password: "pass",
			storage:  &fakeStorage{},
			wantID:   0,
			wantErr:  gophermart.ErrInvalidCredentials,
		},
		{
			name:     "empty password",
			login:    "login",
			password: "",
			storage:  &fakeStorage{},
			wantID:   0,
			wantErr:  gophermart.ErrInvalidCredentials,
		},
		{
			name:     "user not found",
			login:    "login",
			password: "pass",
			storage:  &fakeStorage{err: gophermart.ErrUserNotFound},
			wantID:   0,
			wantErr:  gophermart.ErrInvalidCredentials,
		},
		{
			name:     "user password mismatch",
			login:    "login",
			password: "wrong",
			storage:  &fakeStorage{creds: &model.UserCredentials{PasswordHash: hash}},
			wantID:   0,
			wantErr:  gophermart.ErrInvalidCredentials,
		},
		{
			name:     "user logged in",
			login:    "login",
			password: "pass",
			storage:  &fakeStorage{creds: &model.UserCredentials{ID: 7, PasswordHash: hash}},
			wantID:   7,
			wantErr:  nil,
		},
		{
			name:     "bypass other DB error",
			login:    "login",
			password: "pass",
			storage:  &fakeStorage{err: dbError},
			wantID:   0,
			wantErr:  dbError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New("secret", time.Hour, tt.storage)
			userID, err := a.Login(t.Context(), tt.login, tt.password)
			assert.Equal(t, tt.wantID, userID)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.login, tt.storage.gotLogin)
		})
	}
}

func TestAuth_UserIDContext(t *testing.T) {
	a := New("secret", time.Hour, nil)

	t.Run("user id in context", func(t *testing.T) {
		ctx := a.WithUserID(t.Context(), 42)
		userID := a.UserIDFromContext(ctx)
		assert.Equal(t, model.UserID(42), userID)
	})
	t.Run("no user id in context", func(t *testing.T) {
		userID := a.UserIDFromContext(t.Context())
		assert.Equal(t, model.UserID(0), userID)
	})
}
