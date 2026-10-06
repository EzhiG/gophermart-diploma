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

func (s *PGStorage) Close() error {
	return s.db.Close()
}
