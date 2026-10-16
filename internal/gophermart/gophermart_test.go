package gophermart

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
