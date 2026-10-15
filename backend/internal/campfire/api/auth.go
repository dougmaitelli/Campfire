package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
)

const sessionCookie = "campfire_session"

type authState struct {
	Authenticated bool `json:"authenticated"`
	SetupRequired bool `json:"setup_required"`
	*service.Principal
	CSRFToken string `json:"csrf_token,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type principalKey struct{}

func currentUser(r *http.Request) service.Principal {
	principal, _ := r.Context().Value(principalKey{}).(service.Principal)
	return principal
}

func sessionState(session database.UserSession) authState {
	principal := service.PrincipalFromSession(session)
	return authState{Authenticated: true, Principal: &principal, CSRFToken: session.CSRF, ExpiresAt: session.ExpiresAt}
}

func browserToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}

	return cookie.Value
}

func (a *API) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires int64) {
	cookie := &http.Cookie{Name: sessionCookie, Value: token, Path: "/api", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(a.origin, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: int(service.SessionLifetime.Seconds()), Expires: time.Unix(expires, 0)}
	if token == "" {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
	}

	http.SetCookie(w, cookie)
}

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")

		if a.auth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "administrator authentication is not configured"})
			return
		}

		unsafe := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if unsafe {
			expected := a.origin
			if expected == "" {
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}

				expected = scheme + "://" + r.Host
			}

			if origin := r.Header.Get("Origin"); origin != "" && origin != expected || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin requests are not allowed"})
				return
			}
		}

		public := (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/api/v1/auth/session" || r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/setup")
		if public {
			next.ServeHTTP(w, r)
			return
		}

		session, _, err := a.auth.Session(r.Context(), browserToken(r))
		if err != nil {
			writeAuthError(w, err)
			return
		}

		if unsafe && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid CSRF token; reload and try again"})
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, service.PrincipalFromSession(session))))
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrAuthentication):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in with valid account credentials"})
	case errors.Is(err, service.ErrAdminExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrAuthInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrAuthRate):
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
	default:
		writeError(w, err)
	}
}

func (a *API) authSession(w http.ResponseWriter, r *http.Request) {
	session, _, err := a.auth.Session(r.Context(), browserToken(r))
	if err == nil {
		writeJSON(w, http.StatusOK, sessionState(session))
		return
	}

	if !errors.Is(err, service.ErrAuthentication) {
		writeAuthError(w, err)
		return
	}

	setup, err := a.auth.NeedsSetup(r.Context())
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, authState{SetupRequired: setup})
}

func (a *API) authAttempt(w http.ResponseWriter, r *http.Request, setup bool) {
	address, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		address = r.RemoteAddr
	}

	if !a.auth.AllowAttempt(address) {
		writeAuthError(w, service.ErrAuthRate)
		return
	}

	var input struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		SetupToken string `json:"setup_token,omitempty"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	var session database.UserSession

	var token string
	if setup {
		session, token, err = a.auth.Setup(r.Context(), input.SetupToken, input.Username, input.Password)
	} else {
		session, token, err = a.auth.Login(r.Context(), input.Username, input.Password, browserToken(r))
	}

	if err != nil {
		writeAuthError(w, err)
		return
	}

	session, _, err = a.auth.Session(r.Context(), token)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	a.setSessionCookie(w, r, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, sessionState(session))
}

func (a *API) authSetup(w http.ResponseWriter, r *http.Request) { a.authAttempt(w, r, true) }
func (a *API) authLogin(w http.ResponseWriter, r *http.Request) { a.authAttempt(w, r, false) }

func (a *API) authLogout(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.Logout(r.Context(), browserToken(r)); err != nil {
		writeAuthError(w, err)
		return
	}

	a.setSessionCookie(w, r, "", 0)
	w.WriteHeader(http.StatusNoContent)
}
