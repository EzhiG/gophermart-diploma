package handler

import (
	"net/http"

	"github.com/EzhiG/gophermart-diploma/internal/model"
)

const cookieName = "token"

func setAuthCookie(auth AuthService, w http.ResponseWriter, userID model.UserID) error {
	token, expires, err := auth.BuildToken(userID)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Expires:  expires,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

func getAuthCookie(r *http.Request) string {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}

	return cookie.Value
}

func (h *Handler) NewAuthMiddleware() func(next http.Handler) http.Handler {
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := getAuthCookie(r)
			claims, err := h.auth.ParseToken(token)

			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			if h.auth.NeedsRefresh(claims) {
				err := setAuthCookie(h.auth, w, claims.UserID)
				if err != nil {
					h.logger.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}

			newCtx := h.auth.WithUserID(r.Context(), claims.UserID)
			r = r.WithContext(newCtx)

			next.ServeHTTP(w, r)
		})
	}

	return mw
}
