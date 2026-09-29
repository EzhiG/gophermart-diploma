package config

import (
	"flag"
	"os"
)

type Config struct {
	RunAddress           string
	DatabaseURI          string
	AccrualSystemAddress string
}

func New() *Config {
	cfg := new(Config)

	flag.StringVar(&cfg.RunAddress, "a", "localhost:8080", "http service address")
	flag.StringVar(&cfg.DatabaseURI, "d", "", "database uri")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", "", "accrual system address")
	flag.Parse()

	if envRunAddr, ok := os.LookupEnv("RUN_ADDRESS"); ok {
		cfg.RunAddress = envRunAddr
	}

	if envDatabaseURI, ok := os.LookupEnv("DATABASE_URI"); ok {
		cfg.DatabaseURI = envDatabaseURI
	}

	if envAccrualAddr, ok := os.LookupEnv("ACCRUAL_SYSTEM_ADDRESS"); ok {
		cfg.AccrualSystemAddress = envAccrualAddr
	}

	return cfg
}
