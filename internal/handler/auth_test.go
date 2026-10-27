package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/auth"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestToken(t *testing.T, secret string, ttl time.Duration) string {
	t.Helper()

	token, _, err := auth.New(secret, ttl, nil).BuildToken(testUserID)
	require.NoError(t, err)

	return token
}

func TestHandler_NewAuthMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		buildTokenErr error
		wantStatus    int
		wantNext      bool
		wantRefresh   bool
	}{
		{
			name:       "no cookie",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "garbage token",
			token:      "garbage",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "other secret",
			token:      buildTestToken(t, "other-secret", time.Hour),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "expired token",
			token:      buildTestToken(t, testSecret, -time.Minute),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "fresh token",
			token:      buildTestToken(t, testSecret, time.Hour),
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:        "token needs refresh",
			token:       buildTestToken(t, testSecret, time.Minute),
			wantStatus:  http.StatusOK,
			wantNext:    true,
			wantRefresh: true,
		},
		{
			name:          "refresh build token error",
			token:         buildTestToken(t, testSecret, time.Minute),
			buildTokenErr: errServiceError,
			wantStatus:    http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(nil)
			fa.buildTokenErr = tt.buildTokenErr
			h := New(fa, &fakeMart{}, logger)

			nextCalled := false
			var gotUserID model.UserID
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				gotUserID = fa.UserIDFromContext(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
			if tt.token != "" {
				req.AddCookie(&http.Cookie{Name: cookieName, Value: tt.token})
			}
			rec := httptest.NewRecorder()
			h.NewAuthMiddleware()(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantNext, nextCalled)
			if tt.wantNext {
				assert.Equal(t, testUserID, gotUserID)
			}

			if tt.wantRefresh {
				assertAuthCookie(t, rec, fa, testUserID)
			} else {
				assert.Empty(t, rec.Header().Get("Set-Cookie"))
			}
		})
	}
}
