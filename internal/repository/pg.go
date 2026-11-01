package repository

import (
	"context"
	"errors"

	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/EzhiG/gophermart-diploma/migrations"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

type sqlExecutor interface {
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
}

// PGStorage - хранилище в PostgreSQL.
type PGStorage struct {
	pool *pgxpool.Pool
}

// NewPGStorage подключается к базе по dsn и применяет миграции из пакета migrations.
// Возвращает ошибку, если миграции не применились.
func NewPGStorage(dsn string) (*PGStorage, error) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	storage := &PGStorage{pool}

	if err := storage.applyMigrations(); err != nil {
		pool.Close()
		return nil, err
	}

	return storage, nil
}

func (s *PGStorage) applyMigrations() error {
	ctx := context.Background()
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	dbDriver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		return err
	}
	defer dbDriver.Close()

	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}

// CreateUser создает пользователя и возвращает его UserID.
// Если логин занят, возвращает gophermart.ErrUserExists.
func (s *PGStorage) CreateUser(ctx context.Context, login, passwordHash string) (model.UserID, error) {
	query := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`
	row := s.pool.QueryRow(ctx, query, login, passwordHash)
	var userID model.UserID
	err := row.Scan(&userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return userID, gophermart.ErrUserExists
		}

		return userID, err
	}

	return userID, nil
}

// GetUserCredentials возвращает UserID, логин и хеш пароля по логину.
// Если пользователь не найден, возвращает gophermart.ErrUserNotFound.
func (s *PGStorage) GetUserCredentials(ctx context.Context, login string) (*model.UserCredentials, error) {
	credentials := model.UserCredentials{}
	query := `SELECT id, login, password_hash FROM users WHERE login = $1`
	err := s.pool.QueryRow(ctx, query, login).Scan(&credentials.ID, &credentials.Login, &credentials.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, gophermart.ErrUserNotFound
		}

		return nil, err
	}

	return &credentials, nil
}

// GetUserBalance возвращает баланс пользователя.
// Если пользователь не найден, возвращает gophermart.ErrUserNotFound.
func (s *PGStorage) GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error) {
	row := s.pool.QueryRow(ctx, "SELECT current, withdrawn FROM users WHERE id = $1", userID)
	var balance model.UserBalance

	err := row.Scan(&balance.Current, &balance.Withdrawn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, gophermart.ErrUserNotFound
		}
		return nil, err
	}

	return &balance, nil
}

// CreateUserOrder сохраняет заказ orderNumber за пользователем userID в статусе NEW.
// Возвращает true, если заказ новый, и false, если этот пользователь уже загрузил заказ.
// Если заказ загрузил другой пользователь, возвращает gophermart.ErrOrderAlreadyCreatedByOther.
func (s *PGStorage) CreateUserOrder(ctx context.Context, userID model.UserID, orderNumber string) (bool, error) {
	query := `INSERT INTO orders (user_id, number) VALUES ($1, $2) ON CONFLICT(number) DO NOTHING RETURNING user_id`
	var ownerID model.UserID
	err := s.pool.QueryRow(ctx, query, userID, orderNumber).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ownerID, err = getOrderOwner(ctx, s.pool, orderNumber)
			if err != nil {
				return false, err
			}
			if ownerID == userID {
				return false, nil
			}

			return false, gophermart.ErrOrderAlreadyCreatedByOther
		}
		return false, err
	}

	return true, nil
}

// GetUserOrderList возвращает заказы пользователя.
// Если заказов нет, возвращает nil без ошибки.
func (s *PGStorage) GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error) {
	query := `SELECT number, status, accrual, uploaded_at FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC`
	var orders []model.UserOrderItem
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var order model.UserOrderItem
		err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

// GetUnfinishedOrders возвращает номера до 100 незавершенных заказов всех пользователей.
func (s *PGStorage) GetUnfinishedOrders(ctx context.Context) ([]string, error) {
	orderNumbers := make([]string, 0)
	query := `SELECT number FROM orders WHERE status IN ('NEW', 'PROCESSING') ORDER BY uploaded_at LIMIT 100`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var orderNumber string
		err := rows.Scan(&orderNumber)
		if err != nil {
			return nil, err
		}
		orderNumbers = append(orderNumbers, orderNumber)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orderNumbers, nil
}

// UpdateUserOrderStatus меняет статус незавершенного заказа на status.
// Если заказ не найден или уже в финальном статусе, возвращает gophermart.ErrOrderAlreadyProcessed.
func (s *PGStorage) UpdateUserOrderStatus(ctx context.Context, orderNumber string, status model.OrderStatus) error {
	_, err := updateUserOrder(ctx, s.pool, orderNumber, status, nil)
	return err
}

// SetUserOrderProcessed в одной транзакции переводит заказ в PROCESSED,
// сохраняет accrual и зачисляет accrual на баланс владельца заказа.
// Если заказ не найден или уже в финальном статусе, возвращает gophermart.ErrOrderAlreadyProcessed.
func (s *PGStorage) SetUserOrderProcessed(ctx context.Context, orderNumber string, accrual float64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	userID, err := updateUserOrder(ctx, tx, orderNumber, model.OrderStatusProcessed, &accrual)
	if err != nil {
		return err
	}

	err = fulfillUserBalance(ctx, tx, userID, accrual)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetUserWithdrawals возвращает списания пользователя.
// Если списаний нет, возвращает nil без ошибки.
func (s *PGStorage) GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error) {
	query := `SELECT order_number, sum, processed_at FROM withdrawals WHERE user_id = $1 ORDER BY processed_at DESC`
	var withdrawals []model.UserWithdrawalItem
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var withdrawal model.UserWithdrawalItem
		err := rows.Scan(&withdrawal.OrderNumber, &withdrawal.Sum, &withdrawal.ProcessedAt)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, withdrawal)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return withdrawals, nil
}

// WithdrawUserBalance в одной транзакции уменьшает баланс на sum и сохраняет списание.
// Баланс не уходит ниже нуля.
// Возвращает:
//   - gophermart.ErrInsufficientFunds, если на балансе меньше sum;
//   - gophermart.ErrUserNotFound, если пользователь не найден;
//   - ошибку базы без изменений.
func (s *PGStorage) WithdrawUserBalance(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	err = withdrawUserBalance(ctx, tx, userID, sum)
	if err != nil {
		return err
	}

	err = createWithdrawal(ctx, tx, userID, orderNumber, sum)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Close закрывает подключения к базе.
func (s *PGStorage) Close() {
	s.pool.Close()
}

func createWithdrawal(ctx context.Context, exec sqlExecutor, userID model.UserID, orderNumber string, sum float64) error {
	query := `INSERT INTO withdrawals (user_id, order_number, sum) VALUES ($1, $2, $3)`
	_, err := exec.Exec(ctx, query, userID, orderNumber, sum)
	if err != nil {
		return err
	}

	return nil
}

func withdrawUserBalance(ctx context.Context, exec sqlExecutor, userID model.UserID, sum float64) error {
	var balance float64
	err := exec.QueryRow(ctx, `SELECT current FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gophermart.ErrUserNotFound
		}
		return err
	}
	if balance < sum {
		return gophermart.ErrInsufficientFunds
	}

	query := `UPDATE users SET withdrawn = withdrawn + $1, current = current - $1 WHERE id = $2`
	res, err := exec.Exec(ctx, query, sum, userID)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return gophermart.ErrUserNotFound
	}

	return nil
}

func updateUserOrder(ctx context.Context, exec sqlExecutor, orderNumber string, status model.OrderStatus, accrual *float64) (model.UserID, error) {
	query := `UPDATE orders SET status = $1, accrual = $2 WHERE number = $3 AND status IN ('NEW', 'PROCESSING') RETURNING user_id`
	row := exec.QueryRow(ctx, query, status, accrual, orderNumber)
	var userID model.UserID
	err := row.Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, gophermart.ErrOrderAlreadyProcessed
		}
		return 0, err
	}

	return userID, nil
}

func fulfillUserBalance(ctx context.Context, exec sqlExecutor, userID model.UserID, accrual float64) error {
	query := "UPDATE users SET current = current + $1 WHERE id = $2"
	res, err := exec.Exec(ctx, query, accrual, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return gophermart.ErrUserNotFound
	}

	return nil
}

func getOrderOwner(ctx context.Context, exec sqlExecutor, orderNumber string) (model.UserID, error) {
	query := `SELECT user_id FROM orders WHERE number = $1`
	row := exec.QueryRow(ctx, query, orderNumber)

	var userID model.UserID
	err := row.Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, gophermart.ErrOrderNotFound
		}
		return 0, err
	}

	return userID, nil
}
