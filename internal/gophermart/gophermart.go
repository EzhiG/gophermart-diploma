// Package gophermart содержит бизнес-логику накопительной системы: заказы, баланс,
// списания и синхронизацию заказов с системой расчета.
package gophermart

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/EzhiG/gophermart-diploma/internal/model"
	"go.uber.org/zap"
)

// Storage - хранилище, которое использует Gophermart.
// Контракт методов описан в repository.PGStorage.
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

// Gophermart - сервис накопительной системы.
type Gophermart struct {
	storage              Storage
	accrualSystemAddress string
	logger               *zap.SugaredLogger
	doneCh               chan struct{}
}

// New создает сервис.
// accrualSystemAddress - адрес системы расчета, например http://localhost:8081.
func New(storage Storage, accrualSystemAddress string, logger *zap.SugaredLogger) *Gophermart {
	return &Gophermart{
		storage:              storage,
		accrualSystemAddress: accrualSystemAddress,
		logger:               logger,
		doneCh:               make(chan struct{}),
	}
}

// WaitAccrualSync блокирует вызывающую горутину до выхода из RunAccrualSync.
func (g *Gophermart) WaitAccrualSync() {
	<-g.doneCh
}

// RunAccrualSync синхронизирует незавершенные заказы с системой расчета.
// Блокирует вызывающую горутину до отмены ctx.
// Раз в 1 секунду получает до 100 незавершенных заказов и вызывает SyncOrder для каждого.
// При ответе 429 ждет Retry-After секунд. Если заголовок некорректен, ждет 60 секунд.
// Остальные ошибки записывает в лог и переходит к следующему заказу.
// После выхода разблокирует WaitAccrualSync.
func (g *Gophermart) RunAccrualSync(ctx context.Context) {
	defer close(g.doneCh)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.syncOrders(ctx)
		}
	}

}

func (g *Gophermart) syncOrders(ctx context.Context) {
	orders, err := g.storage.GetUnfinishedOrders(ctx)
	if err != nil {
		g.logger.Errorw("Failed to get unfinished orders", "error", err)
		return
	}

	parallelCh := make(chan struct{}, 5)
	batchCtx, cancelBatch := context.WithCancel(ctx)
	defer cancelBatch()

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	for _, order := range orders {
		select {
		case <-batchCtx.Done():
			return
		case parallelCh <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-parallelCh }()
				err := g.SyncOrder(batchCtx, order)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					if errors.Is(err, ErrAccrualOrderNotFound) {
						g.logger.Debugw("Accrual order not found", "order", order)
						return
					}
					var retryAfterError *AccrualRequestLimitError
					if errors.As(err, &retryAfterError) {
						cancelBatch()
						g.logger.Warnw("Accrual limit reached", "order", order, "message", retryAfterError.Message, "retryAfter", retryAfterError.RetryAfterSeconds)

						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Duration(retryAfterError.RetryAfterSeconds) * time.Second):
							return
						}
					}
					g.logger.Errorw("Failed to sync orders", "error", err, "order", order)
				}
			}()
		}
	}
}

// CreateOrder регистрирует заказ orderNumber для пользователя userID.
// Возвращает:
//   - ErrInvalidOrderNumber, если номер не проходит проверку по алгоритму Луна;
//   - ErrOrderAlreadyCreated, если этот пользователь уже загрузил заказ;
//   - ErrOrderAlreadyCreatedByOther, если заказ загрузил другой пользователь;
//   - другую ошибку без изменений.
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

// GetUserOrderList возвращает заказы пользователя.
// Если заказов нет, возвращает пустой список без ошибки.
func (g *Gophermart) GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error) {
	return g.storage.GetUserOrderList(ctx, userID)
}

// GetUserBalance возвращает баланс пользователя.
// Если пользователь не найден, возвращает ErrUserNotFound.
func (g *Gophermart) GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error) {
	return g.storage.GetUserBalance(ctx, userID)
}

// UserWithdraw списывает sum баллов с баланса пользователя в счет заказа orderNumber.
// Возвращает:
//   - ErrInvalidOrderNumber, если номер не проходит проверку по алгоритму Луна;
//   - ErrWithdrawalNotPositive, если sum меньше или равна нулю;
//   - ErrInsufficientFunds, если на балансе меньше sum баллов;
//   - другую ошибку без изменений.
func (g *Gophermart) UserWithdraw(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error {
	if !checkLuhn(orderNumber) {
		return ErrInvalidOrderNumber
	}
	if sum <= 0 {
		return ErrWithdrawalNotPositive
	}
	return g.storage.WithdrawUserBalance(ctx, userID, orderNumber, sum)
}

// GetUserWithdrawals возвращает списания пользователя.
// Если списаний нет, возвращает пустой список без ошибки.
func (g *Gophermart) GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error) {
	return g.storage.GetUserWithdrawals(ctx, userID)
}

// SyncOrder запрашивает заказ orderNumber в системе расчета и обновляет его статус.
// Тайм-аут запроса - 5 секунд.
// Статус REGISTERED заказ не меняет.
// Статус PROCESSED с начислением зачисляет начисление на баланс владельца заказа.
// Если заказ уже в финальном статусе, ничего не меняет и возвращает nil.
// Возвращает:
//   - ErrAccrualOrderNotFound, если система расчета ответила 204;
//   - *AccrualRequestLimitError, если система расчета ответила 429;
//   - ErrUnknownOrderStatus, если система расчета вернула неизвестный статус;
//   - ошибку HTTP-запроса или хранилища без изменений.
func (g *Gophermart) SyncOrder(ctx context.Context, orderNumber string) error {
	accrualOrder, err := g.getAccrualOrder(ctx, orderNumber)
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

func (g *Gophermart) getAccrualOrder(ctx context.Context, orderNumber string) (*model.AccrualOrder, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var lastErr error
	duration := 100 * time.Millisecond
	for i := range 3 {
		if i > 0 {
			select {
			case <-timeoutCtx.Done():
				return nil, timeoutCtx.Err()
			case <-time.After(duration):
			}
			duration = duration * 2
		}
		req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, g.accrualSystemAddress+"/api/orders/"+orderNumber, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
			lastErr = errors.New(resp.Status)
			_ = resp.Body.Close()
			continue
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
				g.logger.Warnw("Failed to parse Retry-After header", "header", resp.Header, "error", err)
				return nil, NewAccrualRequestLimitError(message, 60)
			}
			return nil, NewAccrualRequestLimitError(message, retryAfter)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, errors.New(resp.Status)
		}

		var accrualOrder model.AccrualOrder
		err = json.NewDecoder(resp.Body).Decode(&accrualOrder)
		if err != nil {
			return nil, err
		}

		return &accrualOrder, nil
	}

	return nil, lastErr
}

func muteProcessedOrderError(err error) error {
	if errors.Is(err, ErrOrderAlreadyProcessed) {
		return nil
	}
	return err
}

func checkLuhn(orderNumber string) bool {
	nDigit := len(orderNumber)
	if nDigit == 0 {
		return false
	}
	sum := 0
	parity := nDigit % 2

	for i := range nDigit {
		if orderNumber[i] < '0' || orderNumber[i] > '9' {
			return false
		}

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
