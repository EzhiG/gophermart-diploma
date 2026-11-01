package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_Parse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want *Config
	}{
		{
			name: "empty",
			args: nil,
			env:  nil,
			want: &Config{
				SecretKey:            secretKey,
				TokenExpirationTime:  tokenExpirationTime,
				RunAddress:           "localhost:8080",
				DatabaseURI:          "",
				AccrualSystemAddress: "",
			},
		},
		{
			name: "flags only",
			args: []string{
				"-a", ":9090",
				"-d", "postgres://user:pass@localhost:5432/db?sslmode=disable",
				"-r", "http://accrual",
			},
			env: nil,
			want: &Config{
				SecretKey:            secretKey,
				TokenExpirationTime:  tokenExpirationTime,
				RunAddress:           ":9090",
				DatabaseURI:          "postgres://user:pass@localhost:5432/db?sslmode=disable",
				AccrualSystemAddress: "http://accrual",
			},
		},
		{
			name: "envs only",
			env: map[string]string{
				"RUN_ADDRESS":            ":4242",
				"ACCRUAL_SYSTEM_ADDRESS": "http://acc",
				"DATABASE_URI":           "postgres://other:pass@localhost:4242/db",
			},
			want: &Config{
				SecretKey:            secretKey,
				TokenExpirationTime:  tokenExpirationTime,
				RunAddress:           ":4242",
				DatabaseURI:          "postgres://other:pass@localhost:4242/db",
				AccrualSystemAddress: "http://acc",
			},
		},
		{
			name: "flags override envs",
			args: []string{
				"-a", ":9090",
				"-d", "postgres://user:pass@localhost:5432/db?sslmode=disable",
				"-r", "http://accrual",
			},
			env: map[string]string{
				"RUN_ADDRESS":            ":4242",
				"ACCRUAL_SYSTEM_ADDRESS": "http://acc",
				"DATABASE_URI":           "postgres://other:pass@localhost:4242/db",
			},
			want: &Config{
				SecretKey:            secretKey,
				TokenExpirationTime:  tokenExpirationTime,
				RunAddress:           ":9090",
				DatabaseURI:          "postgres://user:pass@localhost:5432/db?sslmode=disable",
				AccrualSystemAddress: "http://accrual",
			},
		},
		{
			name: "unspecified flags preserve envs",
			args: []string{"-a", ":9090"},
			env: map[string]string{
				"RUN_ADDRESS":            ":4242",
				"ACCRUAL_SYSTEM_ADDRESS": "http://acc",
				"DATABASE_URI":           "postgres://other:pass@localhost:4242/db",
			},
			want: &Config{
				SecretKey:            secretKey,
				TokenExpirationTime:  tokenExpirationTime,
				RunAddress:           ":9090",
				DatabaseURI:          "postgres://other:pass@localhost:4242/db",
				AccrualSystemAddress: "http://acc",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"RUN_ADDRESS", "DATABASE_URI", "ACCRUAL_SYSTEM_ADDRESS"} {
				t.Setenv(key, "")
				require.NoError(t, os.Unsetenv(key))
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg := parse(tt.args)
			require.Equal(t, tt.want, cfg)
		})
	}
}
