package handler

import "net/http"

const maxRequestBodySize = 1024 * 1024

// MaxBytesMiddleware ограничивает тело запроса размером 1МБ.
// При превышении лимита возвращает *http.MaxBytesError.
func MaxBytesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

		next.ServeHTTP(w, r)
	})
}
