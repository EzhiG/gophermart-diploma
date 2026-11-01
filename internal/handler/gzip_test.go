package handler

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gzipString(t *testing.T, s string) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	zw := gzip.NewWriter(buf)
	_, err := zw.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf
}

func TestHandler_GzipMiddleware_Request(t *testing.T) {
	const testBody = `{"login":"testuser"}`

	tests := []struct {
		name       string
		body       io.Reader
		encoding   string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "plain body",
			body:       strings.NewReader(testBody),
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "gzipped body",
			body:       gzipString(t, testBody),
			encoding:   "gzip",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name:       "broken gzip",
			body:       strings.NewReader(testBody),
			encoding:   "gzip",
			wantStatus: http.StatusBadRequest,
			wantNext:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(newFakeAuth(nil), &fakeMart{}, logger)

			nextCalled := false
			var gotBody string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				gotBody = string(data)
			})

			req := httptest.NewRequest(http.MethodPost, "/", tt.body)
			if tt.encoding != "" {
				req.Header.Set("Content-Encoding", tt.encoding)
			}
			rec := httptest.NewRecorder()
			h.GzipMiddleware(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantNext, nextCalled)
			if tt.wantNext {
				assert.Equal(t, testBody, gotBody)
			}
		})
	}
}

func TestHandler_GzipMiddleware_Response(t *testing.T) {
	const testBody = `{"current":500.5}`

	tests := []struct {
		name           string
		acceptEncoding string
		contentType    string
		status         int
		noWriteHeader  bool
		wantCompressed bool
	}{
		{
			name:           "json, gzip accepted",
			acceptEncoding: "gzip, deflate",
			contentType:    "application/json",
			status:         http.StatusOK,
			wantCompressed: true,
		},
		{
			name:           "json without WriteHeader",
			acceptEncoding: "gzip",
			contentType:    "application/json",
			status:         http.StatusOK,
			noWriteHeader:  true,
			wantCompressed: true,
		},
		{
			name:           "html, gzip accepted",
			acceptEncoding: "gzip",
			contentType:    "text/html; charset=utf-8",
			status:         http.StatusOK,
			wantCompressed: true,
		},
		{
			name:           "gzip not accepted",
			contentType:    "application/json",
			status:         http.StatusOK,
			wantCompressed: false,
		},
		{
			name:           "unsupported content type",
			acceptEncoding: "gzip",
			contentType:    "text/plain",
			status:         http.StatusOK,
			wantCompressed: false,
		},
		{
			name:           "error status",
			acceptEncoding: "gzip",
			contentType:    "application/json",
			status:         http.StatusBadRequest,
			wantCompressed: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(newFakeAuth(nil), &fakeMart{}, logger)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				if !tt.noWriteHeader {
					w.WriteHeader(tt.status)
				}
				_, err := w.Write([]byte(testBody))
				require.NoError(t, err)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			rec := httptest.NewRecorder()
			h.GzipMiddleware(next).ServeHTTP(rec, req)

			assert.Equal(t, tt.status, rec.Code)

			if !tt.wantCompressed {
				assert.Empty(t, rec.Header().Get("Content-Encoding"))
				assert.Equal(t, testBody, rec.Body.String())
				return
			}

			assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
			zr, err := gzip.NewReader(rec.Body)
			require.NoError(t, err)
			data, err := io.ReadAll(zr)
			require.NoError(t, err)
			assert.Equal(t, testBody, string(data))
		})
	}
}
