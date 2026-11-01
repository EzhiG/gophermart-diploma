// Package config читает конфигурацию из флагов командной строки и переменных окружения.
// Явно заданный флаг имеет приоритет над переменной окружения.
package config

import (
	"flag"
	"os"
	"time"
)

// Config - конфигурация сервиса.
type Config struct {
	// RunAddress - адрес HTTP-сервера. Флаг -a, переменная RUN_ADDRESS.
	// По умолчанию localhost:8080.
	RunAddress string
	// DatabaseURI - строка подключения к PostgreSQL. Флаг -d, переменная DATABASE_URI.
	DatabaseURI string
	// AccrualSystemAddress - адрес системы расчета. Флаг -r, переменная ACCRUAL_SYSTEM_ADDRESS.
	AccrualSystemAddress string
	// SecretKey - ключ подписи JWT-токенов. Задается константой.
	SecretKey string
	// TokenExpirationTime - срок действия JWT-токена. Задается константой: 24 часа.
	TokenExpirationTime time.Duration
}

// TODO: по-хорошему, это должно быть в env variables
const secretKey = "MySecretKey"
const tokenExpirationTime = 24 * time.Hour
const defaultRunAddress = "localhost:8080"

// New возвращает конфигурацию из os.Args и переменных окружения.
func New() *Config {
	return parse(os.Args[1:])
}

func parse(args []string) *Config {
	fs := flag.NewFlagSet("gophermart", flag.ExitOnError)
	cfg := new(Config)
	cfg.SecretKey = secretKey
	cfg.TokenExpirationTime = tokenExpirationTime
	cfg.RunAddress = defaultRunAddress

	if envRunAddr, ok := os.LookupEnv("RUN_ADDRESS"); ok {
		cfg.RunAddress = envRunAddr
	}

	if envDatabaseURI, ok := os.LookupEnv("DATABASE_URI"); ok {
		cfg.DatabaseURI = envDatabaseURI
	}

	if envAccrualAddr, ok := os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS"); ok {
		cfg.AccrualSystemAddress = envAccrualAddr
	}

	fs.StringVar(&cfg.RunAddress, "a", cfg.RunAddress, "http service address")
	fs.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "database uri")
	fs.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "accrual system address")
	_ = fs.Parse(args)

	return cfg
}
