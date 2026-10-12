package logger

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

type responseData struct {
	status int
	size   int
}

type logResponseWriter struct {
	http.ResponseWriter
	responseData *responseData
}

func (w *logResponseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.responseData.size += size

	return size, err
}

func (w *logResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.responseData.status = statusCode
}

func NewMiddleware(sugar *zap.SugaredLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		loggedHandler := func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rd := &responseData{
				status: http.StatusOK,
				size:   0,
			}
			lw := &logResponseWriter{
				ResponseWriter: w,
				responseData:   rd,
			}
			next.ServeHTTP(lw, r)
			duration := time.Since(start)

			sugar.Infow(
				"request",
				"uri", r.RequestURI,
				"method", r.Method,
				"status", rd.status,
				"duration", duration,
				"size", rd.size,
			)
		}

		return http.HandlerFunc(loggedHandler)
	}
}
