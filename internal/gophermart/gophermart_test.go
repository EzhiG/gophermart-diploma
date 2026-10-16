package gophermart

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGophermart_CheckLuhn(t *testing.T) {
	tests := []struct {
		name   string
		number []string
		want   bool
	}{
		{
			name:   "valid number",
			number: []string{"4561261212345467", "79927398713", "12345678903"},
			want:   true,
		}, {
			name:   "invalid number",
			number: []string{"79927398710"},
			want:   false,
		}, {
			name:   "empty string",
			number: []string{""},
			want:   false,
		}, {
			name:   "chars",
			number: []string{"a1", "7992739871O"},
			want:   false,
		}, {
			name:   "spaces",
			number: []string{"4561 2612 1234 5467", "7992 7398 713"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, n := range tt.number {
				assert.Equal(t, tt.want, checkLuhn(n), n)
			}
		})
	}
}

type fakeStorage struct {
	err        error
	created    bool
	called     string
	gotStatus  model.OrderStatus
	gotAccrual float64
}

func (s *fakeStorage) GetUserBalance(_ context.Context, _ model.UserID) (*model.UserBalance, error) {
	s.called = "GetUserBalance"
	return nil, s.err
}
func (s *fakeStorage) CreateUserOrder(_ context.Context, _ model.UserID, _ string) (bool, error) {
	s.called = "CreateUserOrder"
	return s.created, s.err
}
func (s *fakeStorage) GetUserOrderList(_ context.Context, _ model.UserID) ([]model.UserOrderItem, error) {
	s.called = "GetUserOrderList"
	return nil, s.err
}
func (s *fakeStorage) UpdateUserOrderStatus(_ context.Context, _ string, status model.OrderStatus) error {
	s.gotStatus = status
	s.called = "UpdateUserOrderStatus"
	return s.err
}
func (s *fakeStorage) SetUserOrderProcessed(_ context.Context, _ string, accrual float64) error {
	s.gotAccrual = accrual
	s.called = "SetUserOrderProcessed"
	return s.err
}
func (s *fakeStorage) GetUserWithdrawals(_ context.Context, _ model.UserID) ([]model.UserWithdrawalItem, error) {
	s.called = "GetUserWithdrawals"
	return nil, s.err
}
func (s *fakeStorage) WithdrawUserBalance(_ context.Context, _ model.UserID, _ string, sum float64) error {
	s.called = "WithdrawUserBalance"
	return s.err
}
func (s *fakeStorage) GetUnfinishedOrders(_ context.Context) ([]string, error) {
	s.called = "GetUnfinishedOrders"
	return nil, s.err
}

var _ Storage = (*fakeStorage)(nil)

const validNumber = "79927398713"

var logger = zap.NewNop().Sugar()
var errStorageError = errors.New("storage error")

func TestGophermart_CreateOrder(t *testing.T) {
	tests := []struct {
		name        string
		orderNumber string
		storage     *fakeStorage
		wantErr     error
		wantCalled  string
	}{
		{
			name:        "invalid order number",
			orderNumber: "79927398710",
			storage:     &fakeStorage{},
			wantErr:     ErrInvalidOrderNumber,
			wantCalled:  "",
		},
		{
			name:        "storage error",
			orderNumber: validNumber,
			storage:     &fakeStorage{err: errStorageError},
			wantErr:     errStorageError,
			wantCalled:  "CreateUserOrder",
		},
		{
			name:        "already created",
			orderNumber: validNumber,
			storage:     &fakeStorage{created: false},
			wantErr:     ErrOrderAlreadyCreated,
			wantCalled:  "CreateUserOrder",
		},
		{
			name:        "created successfully",
			orderNumber: validNumber,
			storage:     &fakeStorage{created: true},
			wantErr:     nil,
			wantCalled:  "CreateUserOrder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(tt.storage, "http://accrual", logger)
			err := g.CreateOrder(t.Context(), 1, tt.orderNumber)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantCalled, tt.storage.called)
		})
	}
}

func TestGophermart_UserWithdraw(t *testing.T) {
	tests := []struct {
		name        string
		orderNumber string
		sum         float64
		storage     *fakeStorage
		wantErr     error
		wantCalled  string
	}{
		{
			name:        "invalid order number",
			orderNumber: "79927398710",
			sum:         10,
			storage:     &fakeStorage{},
			wantErr:     ErrInvalidOrderNumber,
			wantCalled:  "",
		},
		{
			name:        "zero sum",
			orderNumber: validNumber,
			sum:         0,
			storage:     &fakeStorage{},
			wantErr:     ErrWithdrawalNotPositive,
			wantCalled:  "",
		},
		{
			name:        "negative sum",
			orderNumber: validNumber,
			sum:         -10,
			storage:     &fakeStorage{},
			wantErr:     ErrWithdrawalNotPositive,
			wantCalled:  "",
		},
		{
			name:        "not enough money",
			orderNumber: validNumber,
			sum:         10.10,
			storage: &fakeStorage{
				err: ErrInsufficientFunds,
			},
			wantErr:    ErrInsufficientFunds,
			wantCalled: "WithdrawUserBalance",
		},
		{
			name:        "other storage error as is",
			orderNumber: validNumber,
			sum:         10.10,
			storage:     &fakeStorage{err: errStorageError},
			wantErr:     errStorageError,
			wantCalled:  "WithdrawUserBalance",
		},
		{
			name:        "withdrew successfully",
			orderNumber: validNumber,
			sum:         10.10,
			storage:     &fakeStorage{},
			wantErr:     nil,
			wantCalled:  "WithdrawUserBalance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(tt.storage, "http://accrual", logger)
			err := g.UserWithdraw(t.Context(), 1, tt.orderNumber, tt.sum)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantCalled, tt.storage.called)
		})
	}
}

func newAccrualServer(t *testing.T, status int, retryAfter, body string) *httptest.Server {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestGophermart_getAccrualOrder(t *testing.T) {
	accrual := 500.5
	tests := []struct {
		name       string
		status     int
		retryAfter string
		body       string
		wantOrder  *model.AccrualOrder
		wantErr    error
		wantAnyErr bool
	}{
		{
			name:   "return order",
			status: http.StatusOK,
			body:   `{"order":"79927398713","status":"PROCESSED","accrual":500.5}`,
			wantOrder: &model.AccrualOrder{
				Order:   "79927398713",
				Status:  model.AccrualOrderStatusProcessed,
				Accrual: &accrual,
			},
		},
		{
			name:    "not found",
			status:  http.StatusNoContent,
			wantErr: ErrAccrualOrderNotFound,
		},
		{
			name:       "unknown error",
			status:     http.StatusInternalServerError,
			wantAnyErr: true,
		},
		{
			name:       "bad json",
			status:     http.StatusOK,
			body:       "{",
			wantAnyErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newAccrualServer(t, tt.status, "", tt.body)
			accrualOrder, err := getAccrualOrder(t.Context(), srv.URL, validNumber)
			if tt.wantAnyErr {
				assert.Error(t, err)
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantOrder, accrualOrder)
		})
	}

	limitTests := []struct {
		name           string
		retryAfter     string
		wantRetryAfter int
	}{
		{
			name:           "retry after header",
			retryAfter:     "5",
			wantRetryAfter: 5,
		},
		{
			name:           "retry fallback",
			retryAfter:     "",
			wantRetryAfter: 60,
		},
		{
			name:           "retry unknown header",
			retryAfter:     "abc",
			wantRetryAfter: 60,
		},
	}
	for _, tt := range limitTests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newAccrualServer(t, http.StatusTooManyRequests, tt.retryAfter, "some message")
			accrualOrder, err := getAccrualOrder(t.Context(), srv.URL, validNumber)
			var errLimit *AccrualRequestLimitError
			require.ErrorAs(t, err, &errLimit)
			assert.Equal(t, tt.wantRetryAfter, errLimit.RetryAfterSeconds)
			assert.Equal(t, "some message", errLimit.Message)
			assert.Nil(t, accrualOrder)
		})
	}

	t.Run("correct url", func(t *testing.T) {
		var gotMethod string
		var gotPath string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"order":"` + validNumber + `","status":"PROCESSED","accrual":500.5}`))
		}))
		t.Cleanup(server.Close)
		_, err := getAccrualOrder(t.Context(), server.URL, validNumber)
		require.NoError(t, err)
		assert.Equal(t, http.MethodGet, gotMethod)
		assert.Equal(t, "/api/orders/"+validNumber, gotPath)
	})
}

func TestGophermart_SyncOrder(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		storage     *fakeStorage
		wantErr     error
		wantCalled  string
		wantStatus  model.OrderStatus
		wantAccrual float64
	}{
		{
			name:       "accrual not found",
			status:     http.StatusNoContent,
			storage:    &fakeStorage{},
			wantErr:    ErrAccrualOrderNotFound,
			wantCalled: "",
		},
		{
			name:       "accrual registered",
			status:     http.StatusOK,
			storage:    &fakeStorage{},
			body:       `{"order":"79927398713","status":"REGISTERED","accrual":500.5}`,
			wantCalled: "",
		},
		{
			name:       "accrual invalid",
			status:     http.StatusOK,
			storage:    &fakeStorage{},
			body:       `{"order":"79927398713","status":"INVALID","accrual":500.5}`,
			wantCalled: "UpdateUserOrderStatus",
			wantStatus: model.OrderStatusInvalid,
		},
		{
			name:       "accrual processing",
			status:     http.StatusOK,
			storage:    &fakeStorage{},
			body:       `{"order":"79927398713","status":"PROCESSING","accrual":500.5}`,
			wantCalled: "UpdateUserOrderStatus",
			wantStatus: model.OrderStatusProcessing,
		},
		{
			name:       "accrual processed no sum",
			status:     http.StatusOK,
			storage:    &fakeStorage{},
			body:       `{"order":"79927398713","status":"PROCESSED"}`,
			wantCalled: "UpdateUserOrderStatus",
			wantStatus: model.OrderStatusProcessed,
		},
		{
			name:        "accrual processed with sum",
			status:      http.StatusOK,
			storage:     &fakeStorage{},
			body:        `{"order":"79927398713","status":"PROCESSED","accrual": 500.5}`,
			wantCalled:  "SetUserOrderProcessed",
			wantAccrual: 500.5,
		},
		{
			name:       "accrual unknown order status",
			status:     http.StatusOK,
			storage:    &fakeStorage{},
			body:       `{"order":"79927398713","status":"UNKNOWN"}`,
			wantCalled: "",
			wantErr:    ErrUnknownOrderStatus,
		},
		{
			name:   "mute order already processed",
			status: http.StatusOK,
			storage: &fakeStorage{
				err: ErrOrderAlreadyProcessed,
			},
			body:        `{"order":"79927398713","status":"PROCESSED", "accrual": 500.5}`,
			wantCalled:  "SetUserOrderProcessed",
			wantAccrual: 500.5,
		},
		{
			name:       "storage error",
			status:     http.StatusOK,
			storage:    &fakeStorage{err: errStorageError},
			body:       `{"order":"79927398713","status":"PROCESSING","accrual":500.5}`,
			wantCalled: "UpdateUserOrderStatus",
			wantStatus: model.OrderStatusProcessing,
			wantErr:    errStorageError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newAccrualServer(t, tt.status, "", tt.body)
			g := New(tt.storage, srv.URL, logger)
			err := g.SyncOrder(t.Context(), validNumber)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantCalled, tt.storage.called)
			assert.Equal(t, tt.wantStatus, tt.storage.gotStatus)
			assert.Equal(t, tt.wantAccrual, tt.storage.gotAccrual)
		})
	}
}

func TestGophermart_PassThrough(t *testing.T) {
	tests := []struct {
		name       string
		wantErr    error
		wantCalled string
		call       func(ctx context.Context, g *Gophermart) error
	}{
		{
			name:       "GetUserOrderList",
			wantErr:    errStorageError,
			wantCalled: "GetUserOrderList",
			call: func(ctx context.Context, g *Gophermart) error {
				_, err := g.GetUserOrderList(ctx, 1)
				return err
			},
		},
		{
			name:       "GetUserBalance",
			wantErr:    errStorageError,
			wantCalled: "GetUserBalance",
			call: func(ctx context.Context, g *Gophermart) error {
				_, err := g.GetUserBalance(ctx, 1)
				return err
			},
		},
		{
			name:       "GetUserWithdrawals",
			wantErr:    errStorageError,
			wantCalled: "GetUserWithdrawals",
			call: func(ctx context.Context, g *Gophermart) error {
				_, err := g.GetUserWithdrawals(ctx, 1)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &fakeStorage{err: errStorageError}
			g := New(storage, "http://accrual", logger)
			err := tt.call(t.Context(), g)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantCalled, storage.called)
		})
	}
}
