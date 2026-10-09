package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/EzhiG/gophermart-diploma/internal/gophermart"
	"github.com/EzhiG/gophermart-diploma/internal/model"
	"github.com/EzhiG/gophermart-diploma/migrations"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type sqlExecutor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type PGStorage struct {
	db *sql.DB
}

func NewPGStorage(dsn string) (*PGStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	storage := &PGStorage{db: db}

	if err := storage.applyMigrations(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *PGStorage) applyMigrations() error {
	dbDriver, err := postgres.WithInstance(s.db, &postgres.Config{})
	if err != nil {
		return err
	}

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

func (s *PGStorage) CreateUser(ctx context.Context, login, passwordHash string) (model.UserID, error) {
	query := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`
	row := s.db.QueryRowContext(ctx, query, login, passwordHash)
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

func (s *PGStorage) GetUserCredentials(ctx context.Context, login string) (*model.UserCredentials, error) {
	credentials := model.UserCredentials{}
	query := `SELECT id, login, password_hash FROM users WHERE login = $1`
	err := s.db.QueryRowContext(ctx, query, login).Scan(&credentials.ID, &credentials.Login, &credentials.PasswordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gophermart.ErrUserNotFound
		}

		return nil, err
	}

	return &credentials, nil
}

func (s *PGStorage) GetUserBalance(ctx context.Context, userID model.UserID) (*model.UserBalance, error) {
	row := s.db.QueryRowContext(ctx, "SELECT current, withdrawn FROM users WHERE id = $1", userID)
	var balance model.UserBalance

	err := row.Scan(&balance.Current, &balance.Withdrawn)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, gophermart.ErrUserNotFound
		}
		return nil, err
	}

	return &balance, nil
}

func (s *PGStorage) CreateUserOrder(ctx context.Context, userID model.UserID, orderNumber string) (bool, error) {
	query := `INSERT INTO orders (user_id, number) VALUES ($1, $2) ON CONFLICT(number) DO NOTHING RETURNING user_id`
	var ownerID model.UserID
	err := s.db.QueryRowContext(ctx, query, userID, orderNumber).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ownerID, err = getOrderOwner(ctx, s.db, orderNumber)
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

func (s *PGStorage) GetUserOrderList(ctx context.Context, userID model.UserID) ([]model.UserOrderItem, error) {
	query := `SELECT number, status, accrual, uploaded_at FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC`
	var orders []model.UserOrderItem
	rows, err := s.db.QueryContext(ctx, query, userID)
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

func (s *PGStorage) UpdateUserOrderStatus(ctx context.Context, orderNumber string, status model.OrderStatus) error {
	_, err := updateUserOrder(ctx, s.db, orderNumber, status, nil)
	return err
}

func (s *PGStorage) SetUserOrderProcessed(ctx context.Context, orderNumber string, accrual float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	userID, err := updateUserOrder(ctx, tx, orderNumber, model.OrderStatusProcessed, &accrual)
	if err != nil {
		return err
	}

	err = fulfillUserBalance(ctx, tx, userID, accrual)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *PGStorage) GetUserWithdrawals(ctx context.Context, userID model.UserID) ([]model.UserWithdrawalItem, error) {
	query := `SELECT order_number, sum, processed_at FROM withdrawals WHERE user_id = $1 ORDER BY processed_at DESC`
	var withdrawals []model.UserWithdrawalItem
	rows, err := s.db.QueryContext(ctx, query, userID)
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

func (s *PGStorage) WithdrawUserBalance(ctx context.Context, userID model.UserID, orderNumber string, sum float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	err = createWithdrawal(ctx, tx, userID, orderNumber, sum)
	if err != nil {
		return err
	}

	err = withdrawUserBalance(ctx, tx, userID, sum)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *PGStorage) Close() error {
	return s.db.Close()
}

func createWithdrawal(ctx context.Context, exec sqlExecutor, userID model.UserID, orderNumber string, sum float64) error {
	query := `INSERT INTO withdrawals (user_id, order_number, sum) VALUES ($1, $2, $3)`
	_, err := exec.ExecContext(ctx, query, userID, orderNumber, sum)
	if err != nil {
		return err
	}

	return nil
}

func withdrawUserBalance(ctx context.Context, exec sqlExecutor, userID model.UserID, sum float64) error {
	query := `UPDATE users SET withdrawn = withdrawn + $1, current = current - $1 WHERE id = $2`
	res, err := exec.ExecContext(ctx, query, sum, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.CheckViolation && pgErr.ConstraintName == "users_current_check" {
			return gophermart.ErrInsufficientFunds
		}

		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return gophermart.ErrUserNotFound
	}

	return nil
}

func updateUserOrder(ctx context.Context, exec sqlExecutor, orderNumber string, status model.OrderStatus, accrual *float64) (model.UserID, error) {
	query := `UPDATE orders SET status = $1, accrual = $2 WHERE number = $3 AND status IN ('NEW', 'PROCESSING') RETURNING user_id`
	row := exec.QueryRowContext(ctx, query, status, accrual, orderNumber)
	var userID model.UserID
	err := row.Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, gophermart.ErrOrderNotFound
		}
		return 0, err
	}

	return userID, nil
}

func fulfillUserBalance(ctx context.Context, exec sqlExecutor, userID model.UserID, accrual float64) error {
	query := "UPDATE users SET current = current + $1 WHERE id = $2"
	res, err := exec.ExecContext(ctx, query, accrual, userID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return gophermart.ErrUserNotFound
	}

	return nil
}

func getOrderOwner(ctx context.Context, exec sqlExecutor, orderNumber string) (model.UserID, error) {
	query := `SELECT user_id FROM orders WHERE number = $1`
	row := exec.QueryRowContext(ctx, query, orderNumber)

	var userID model.UserID
	err := row.Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, gophermart.ErrOrderNotFound
		}
		return 0, err
	}

	return userID, nil
}
