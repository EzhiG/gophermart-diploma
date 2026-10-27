package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/auth"
	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testSecret               = "test-secret"
	testUserID  model.UserID = 42
	validBody                = `{"login":"testuser","password":"testpass"}`
	validNumber              = "12345678903"
)

var (
	logger          = zap.NewNop().Sugar()
	errServiceError = errors.New("service error")
)

type fakeMart struct {
	err         error
	orders      []model.UserOrderItem
	balance     *model.UserBalance
	withdrawals []model.UserWithdrawalItem
	called      string
	gotUserID   model.UserID
	gotNumber   string
	gotSum      float64
}

func (m *fakeMart) CreateOrder(_ context.Context, userID model.UserID, orderNumber string) error {
	m.called = "CreateOrder"
	m.gotUserID = userID
	m.gotNumber = orderNumber

	return m.err
}

func (m *fakeMart) GetUserOrderList(_ context.Context, userID model.UserID) ([]model.UserOrderItem, error) {
	m.called = "GetUserOrderList"
	m.gotUserID = userID

	return m.orders, m.err
}

func (m *fakeMart) GetUserBalance(_ context.Context, userID model.UserID) (*model.UserBalance, error) {
	m.called = "GetUserBalance"
	m.gotUserID = userID

	return m.balance, m.err
}

func (m *fakeMart) UserWithdraw(_ context.Context, userID model.UserID, orderNumber string, sum float64) error {
	m.called = "UserWithdraw"
	m.gotNumber = orderNumber
	m.gotUserID = userID
	m.gotSum = sum

	return m.err
}

func (m *fakeMart) GetUserWithdrawals(_ context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error) {
	m.called = "GetUserWithdrawals"
	m.gotUserID = userID

	return m.withdrawals, m.err
}

type fakeAuth struct {
	*auth.Auth
	userID        model.UserID
	err           error
	buildTokenErr error
	gotLogin      string
	gotPassword   string
}

func newFakeAuth(err error) *fakeAuth {
	return &fakeAuth{
		Auth:   auth.New(testSecret, time.Hour, nil),
		userID: testUserID,
		err:    err,
	}
}

func (a *fakeAuth) Login(_ context.Context, login, password string) (model.UserID, error) {
	a.gotLogin = login
	a.gotPassword = password

	return a.userID, a.err
}

func (a *fakeAuth) Register(_ context.Context, login, password string) (model.UserID, error) {
	a.gotLogin = login
	a.gotPassword = password

	return a.userID, a.err
}

func (a *fakeAuth) BuildToken(userID model.UserID) (string, time.Time, error) {
	if a.buildTokenErr != nil {
		return "", time.Time{}, a.buildTokenErr
	}

	return a.Auth.BuildToken(userID)
}

var _ MartService = (*fakeMart)(nil)
var _ AuthService = (*fakeAuth)(nil)

func assertAuthCookie(t *testing.T, rec *httptest.ResponseRecorder, a AuthService, wantUserID model.UserID) {
	t.Helper()

	res := rec.Result()
	defer res.Body.Close()

	for _, cookie := range res.Cookies() {
		if cookie.Name == cookieName {
			claims, err := a.ParseToken(cookie.Value)
			require.NoError(t, err)
			assert.Equal(t, wantUserID, claims.UserID)
			return
		}
	}

	t.Errorf("cookie %q not set", cookieName)
}

func newUserRequest(a AuthService, anonymous bool, method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	if anonymous {
		return req
	}

	return req.WithContext(a.WithUserID(req.Context(), testUserID))
}

func TestHandler_APIPostUserRegister(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		err           error
		buildTokenErr error
		wantStatus    int
		wantCalled    bool
	}{
		{
			name:       "bad body",
			wantStatus: http.StatusBadRequest,
			body:       `{`,
			wantCalled: false,
		},
		{
			name:       "invalid credentials",
			body:       validBody,
			wantStatus: http.StatusUnauthorized,
			wantCalled: true,
			err:        gophermart.ErrInvalidCredentials,
		},
		{
			name:       "user exists",
			body:       validBody,
			wantStatus: http.StatusConflict,
			wantCalled: true,
			err:        gophermart.ErrUserExists,
		},
		{
			name:       "too long password",
			body:       validBody,
			wantStatus: http.StatusBadRequest,
			wantCalled: true,
			err:        gophermart.ErrPasswordTooLong,
		},
		{
			name:       "unknown error",
			body:       validBody,
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
			err:        errServiceError,
		},
		{
			name:          "build token error",
			body:          validBody,
			buildTokenErr: errServiceError,
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
		{
			name:       "successful",
			body:       validBody,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(tt.err)
			fa.buildTokenErr = tt.buildTokenErr
			h := New(fa, &fakeMart{}, logger)
			req := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.APIPostUserRegister(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCalled {
				assert.Equal(t, "testuser", fa.gotLogin)
				assert.Equal(t, "testpass", fa.gotPassword)
			} else {
				assert.Empty(t, fa.gotLogin)
			}

			if tt.wantStatus == http.StatusOK {
				assertAuthCookie(t, rec, fa, testUserID)
			}
		})
	}
}

func TestHandler_APIPostUserLogin(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		err           error
		buildTokenErr error
		wantStatus    int
		wantCalled    bool
	}{
		{
			name:       "bad body",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "user not found",
			body:       validBody,
			err:        gophermart.ErrUserNotFound,
			wantStatus: http.StatusUnauthorized,
			wantCalled: true,
		},
		{
			name:       "invalid credentials",
			body:       validBody,
			err:        gophermart.ErrInvalidCredentials,
			wantStatus: http.StatusUnauthorized,
			wantCalled: true,
		},
		{
			name:       "unknown error",
			body:       validBody,
			err:        errServiceError,
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
		{
			name:          "build token error",
			body:          validBody,
			buildTokenErr: errServiceError,
			wantStatus:    http.StatusInternalServerError,
			wantCalled:    true,
		},
		{
			name:       "successful",
			body:       validBody,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(tt.err)
			fa.buildTokenErr = tt.buildTokenErr
			h := New(fa, &fakeMart{}, logger)
			req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.APIPostUserLogin(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCalled {
				assert.Equal(t, "testuser", fa.gotLogin)
				assert.Equal(t, "testpass", fa.gotPassword)
			} else {
				assert.Empty(t, fa.gotLogin)
			}

			if tt.wantStatus == http.StatusOK {
				assertAuthCookie(t, rec, fa, testUserID)
			}
		})
	}
}

func TestHandler_APIPostUserOrder(t *testing.T) {
	tests := []struct {
		name       string
		anonymous  bool
		body       io.Reader
		err        error
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "no user",
			anonymous:  true,
			body:       strings.NewReader(validNumber),
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "body error",
			body:       iotest.ErrReader(errServiceError),
			wantStatus: http.StatusInternalServerError,
			wantCalled: false,
		},
		{
			name:       "empty body",
			body:       strings.NewReader("  \n"),
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "invalid order number",
			body:       strings.NewReader(validNumber),
			err:        gophermart.ErrInvalidOrderNumber,
			wantStatus: http.StatusUnprocessableEntity,
			wantCalled: true,
		},
		{
			name:       "created by other user",
			body:       strings.NewReader(validNumber),
			err:        gophermart.ErrOrderAlreadyCreatedByOther,
			wantStatus: http.StatusConflict,
			wantCalled: true,
		},
		{
			name:       "already created by this user",
			body:       strings.NewReader(validNumber),
			err:        gophermart.ErrOrderAlreadyCreated,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "unknown error",
			body:       strings.NewReader(validNumber),
			err:        errServiceError,
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
		{
			name:       "accepted, number trimmed",
			body:       strings.NewReader(" " + validNumber + "\n"),
			wantStatus: http.StatusAccepted,
			wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(nil)
			fm := &fakeMart{err: tt.err}
			h := New(fa, fm, logger)
			req := newUserRequest(fa, tt.anonymous, http.MethodPost, "/api/user/orders", tt.body)
			rec := httptest.NewRecorder()
			h.APIPostUserOrder(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if !tt.wantCalled {
				assert.Empty(t, fm.called)
				return
			}
			assert.Equal(t, "CreateOrder", fm.called)
			assert.Equal(t, testUserID, fm.gotUserID)
			assert.Equal(t, validNumber, fm.gotNumber)
		})
	}
}

func TestHandler_APIGetUserOrders(t *testing.T) {
	accrual := 500.5
	uploadedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		anonymous  bool
		orders     []model.UserOrderItem
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "no user",
			anonymous:  true,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "service error",
			err:        errServiceError,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "no orders",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "orders",
			orders: []model.UserOrderItem{
				{Number: "9278923470", Status: model.OrderStatusProcessed, Accrual: &accrual, UploadedAt: uploadedAt},
				{Number: validNumber, Status: model.OrderStatusNew, UploadedAt: uploadedAt},
			},
			wantStatus: http.StatusOK,
			wantBody: `[
				{"number":"9278923470","status":"PROCESSED","accrual":500.5,"uploaded_at":"2026-10-01T12:00:00Z"},
				{"number":"12345678903","status":"NEW","uploaded_at":"2026-10-01T12:00:00Z"}
			]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(nil)
			fm := &fakeMart{orders: tt.orders, err: tt.err}
			h := New(fa, fm, logger)
			req := newUserRequest(fa, tt.anonymous, http.MethodGet, "/api/user/orders", nil)
			rec := httptest.NewRecorder()
			h.APIGetUserOrders(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.anonymous {
				assert.Empty(t, fm.called)
			} else {
				assert.Equal(t, testUserID, fm.gotUserID)
			}

			if tt.wantBody != "" {
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				assert.JSONEq(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestHandler_APIGetUserBalance(t *testing.T) {
	tests := []struct {
		name       string
		anonymous  bool
		balance    *model.UserBalance
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "no user",
			anonymous:  true,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "service error",
			err:        errServiceError,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "return balance",
			balance:    &model.UserBalance{Current: 500.5, Withdrawn: 42},
			wantStatus: http.StatusOK,
			wantBody:   `{"current":500.5,"withdrawn":42}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(nil)
			fm := &fakeMart{balance: tt.balance, err: tt.err}
			h := New(fa, fm, logger)
			req := newUserRequest(fa, tt.anonymous, http.MethodGet, "/api/user/balance", nil)
			rec := httptest.NewRecorder()
			h.APIGetUserBalance(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.anonymous {
				assert.Empty(t, fm.called)
			} else {
				assert.Equal(t, testUserID, fm.gotUserID)
			}

			if tt.wantBody != "" {
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				assert.JSONEq(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestHandler_APIPostUserWithdraw(t *testing.T) {
	body := `{"order":"` + validNumber + `","sum":751.5}`

	tests := []struct {
		name       string
		anonymous  bool
		body       string
		err        error
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "no user",
			anonymous:  true,
			body:       body,
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "bad body",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
			wantCalled: false,
		},
		{
			name:       "invalid order number",
			body:       body,
			err:        gophermart.ErrInvalidOrderNumber,
			wantStatus: http.StatusUnprocessableEntity,
			wantCalled: true,
		},
		{
			name:       "sum not positive",
			body:       body,
			err:        gophermart.ErrWithdrawalNotPositive,
			wantStatus: http.StatusBadRequest,
			wantCalled: true,
		},
		{
			name:       "insufficient funds",
			body:       body,
			err:        gophermart.ErrInsufficientFunds,
			wantStatus: http.StatusPaymentRequired,
			wantCalled: true,
		},
		{
			name:       "unknown error",
			body:       body,
			err:        errServiceError,
			wantStatus: http.StatusInternalServerError,
			wantCalled: true,
		},
		{
			name:       "successful",
			body:       body,
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(nil)
			fm := &fakeMart{err: tt.err}
			h := New(fa, fm, logger)
			req := newUserRequest(fa, tt.anonymous, http.MethodPost, "/api/user/balance/withdraw", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.APIPostUserWithdraw(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if !tt.wantCalled {
				assert.Empty(t, fm.called)
				return
			}
			assert.Equal(t, "UserWithdraw", fm.called)
			assert.Equal(t, testUserID, fm.gotUserID)
			assert.Equal(t, validNumber, fm.gotNumber)
			assert.InDelta(t, 751.5, fm.gotSum, 1e-9)
		})
	}
}

func TestHandler_APIGetUserWithdrawals(t *testing.T) {
	processedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		anonymous   bool
		withdrawals []model.UserWithdrawalItem
		err         error
		wantStatus  int
		wantBody    string
	}{
		{
			name:       "no user",
			anonymous:  true,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "service error",
			err:        errServiceError,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "no withdrawals",
			wantStatus: http.StatusNoContent,
		},
		{
			name: "withdrawals",
			withdrawals: []model.UserWithdrawalItem{
				{OrderNumber: validNumber, Sum: 500.5, ProcessedAt: processedAt},
			},
			wantStatus: http.StatusOK,
			wantBody:   `[{"order":"12345678903","sum":500.5,"processed_at":"2026-10-01T12:00:00Z"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := newFakeAuth(nil)
			fm := &fakeMart{withdrawals: tt.withdrawals, err: tt.err}
			h := New(fa, fm, logger)
			req := newUserRequest(fa, tt.anonymous, http.MethodGet, "/api/user/withdrawals", nil)
			rec := httptest.NewRecorder()
			h.APIGetUserWithdrawals(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.anonymous {
				assert.Empty(t, fm.called)
			} else {
				assert.Equal(t, testUserID, fm.gotUserID)
			}

			if tt.wantBody != "" {
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
				assert.JSONEq(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}
