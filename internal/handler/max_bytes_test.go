package handler

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxBytesMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		bodySize    int
		wantTooLong bool
	}{
		{
			name:        "body at limit",
			bodySize:    maxRequestBodySize,
			wantTooLong: false,
		},
		{
			name:        "body over limit",
			bodySize:    maxRequestBodySize + 1,
			wantTooLong: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			var readSize int
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				readErr = err
				readSize = len(data)
			})

			body := strings.Repeat("a", tt.bodySize)
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			MaxBytesMiddleware(next).ServeHTTP(w, r)

			if !tt.wantTooLong {
				require.NoError(t, readErr)
				assert.Equal(t, tt.bodySize, readSize)
				return
			}

			var maxBytesErr *http.MaxBytesError
			require.True(t, errors.As(readErr, &maxBytesErr))
			assert.Equal(t, int64(maxRequestBodySize), maxBytesErr.Limit)
		})
	}
}
