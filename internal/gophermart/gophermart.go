package gophermart

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/model"
)

type Storage interface {
	GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error)
	CreateUserOrder(ctx context.Context, userID model.UserID, orderNumber string) (bool, error)
	GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error)
	UpdateUserOrderStatus(ctx context.Context, orderNumber string, status model.OrderStatus) error
	SetUserOrderProcessed(ctx context.Context, orderNumber string, accrual float64) error
	GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error)
	WithdrawUserBalance(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error
	GetUnfinishedOrders(ctx context.Context) ([]string, error)
}
type Gophermart struct {
	storage              Storage
	accrualSystemAddress string
}

func New(storage Storage, accrualSystemAddress string) *Gophermart {
	return &Gophermart{storage, accrualSystemAddress}
}

func (g *Gophermart) CreateOrder(ctx context.Context, userID model.UserID, orderNumber string) error {
	if !checkLuhn(orderNumber) {
		return ErrInvalidOrderNumber
	}
	ok, err := g.storage.CreateUserOrder(ctx, userID, orderNumber)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOrderAlreadyCreated
	}

	return nil
}

func (g *Gophermart) GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error) {
	return g.storage.GetUserOrderList(ctx, userID)
}

func (g *Gophermart) GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error) {
	return g.storage.GetUserBalance(ctx, userID)
}

func (g *Gophermart) UserWithdraw(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error {
	if !checkLuhn(orderNumber) {
		return ErrInvalidOrderNumber
	}
	if sum <= 0 {
		return ErrWithdrawalNotPositive
	}
	return g.storage.WithdrawUserBalance(ctx, userID, orderNumber, sum)
}

func (g *Gophermart) GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error) {
	return g.storage.GetUserWithdrawals(ctx, userID)
}

func (g *Gophermart) SyncOrder(ctx context.Context, orderNumber string) error {
	accrualOrder, err := getAccrualOrder(ctx, g.accrualSystemAddress, orderNumber)
	if err != nil {
		return err
	}

	if accrualOrder.Status == model.AccrualOrderStatusRegistered {
		return nil
	}

	if accrualOrder.Status == model.AccrualOrderStatusInvalid {
		return muteProcessedOrderError(g.storage.UpdateUserOrderStatus(ctx, orderNumber, model.OrderStatusInvalid))
	}

	if accrualOrder.Status == model.AccrualOrderStatusProcessing {
		return muteProcessedOrderError(g.storage.UpdateUserOrderStatus(ctx, orderNumber, model.OrderStatusProcessing))
	}

	if accrualOrder.Status == model.AccrualOrderStatusProcessed {
		if accrualOrder.Accrual == nil {
			return muteProcessedOrderError(g.storage.UpdateUserOrderStatus(ctx, orderNumber, model.OrderStatusProcessed))
		}

		return muteProcessedOrderError(g.storage.SetUserOrderProcessed(ctx, orderNumber, *accrualOrder.Accrual))
	}

	return ErrUnknownOrderStatus
}

func muteProcessedOrderError(err error) error {
	if errors.Is(err, ErrOrderAlreadyProcessed) {
		return nil
	}
	return err
}

func getAccrualOrder(ctx context.Context, address, orderNumber string) (*model.AccrualOrder, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, address+"/api/orders/"+orderNumber, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, ErrAccrualOrderNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		text, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		message := string(text)
		retryAfter, err := strconv.Atoi(resp.Header.Get("Retry-After"))
		if err != nil {
			return nil, NewAccrualRequestLimitError(message, 60)
		}
		return nil, NewAccrualRequestLimitError(message, retryAfter)
	}

	var accrualOrder model.AccrualOrder
	err = json.NewDecoder(resp.Body).Decode(&accrualOrder)
	if err != nil {
		return nil, err
	}

	return &accrualOrder, nil
}

var digitsRe = regexp.MustCompile(`\d+`)

func checkLuhn(orderNumber string) bool {
	if !digitsRe.MatchString(orderNumber) {
		return false
	}

	sum := 0
	nDigit := len(orderNumber)
	parity := nDigit % 2

	for i := range nDigit {
		digit := int(orderNumber[i] - '0')
		if i%2 == parity {
			digit = digit * 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}

	return sum%10 == 0
}
