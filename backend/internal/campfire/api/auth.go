package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

const sessionCookie = "campfire_session"

type authState struct {
	Authenticated bool `json:"authenticated"`
	SetupRequired bool `json:"setup_required"`
	*service.Principal
	CSRFToken   string       `json:"csrf_token,omitempty"`
	ExpiresAt   int64        `json:"expires_at,omitempty"`
	LocalLogin  bool         `json:"local_login"`
	OIDCEnabled bool         `json:"oidc_enabled"`
	ViewAs      *viewAsState `json:"view_as,omitempty"`
	ViewAsEnded bool         `json:"view_as_ended,omitempty"`
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

// requestPrincipal is also used by long-running responses to recheck revocation.
func (a *API) requestPrincipal(r *http.Request) (service.Principal, error) {
	if values, exists := r.Header["Authorization"]; exists {
		if len(values) != 1 {
			return service.Principal{}, service.ErrAuthentication
		}

		parts := strings.Fields(values[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return service.Principal{}, service.ErrAuthentication
		}

		return a.auth.AuthenticateAPIToken(r.Context(), parts[1])
	}

	session, _, err := a.auth.Session(r.Context(), browserToken(r))
	if err != nil {
		return service.Principal{}, err
	}

	principal, _, err := a.browserPrincipal(r, session)

	return principal, err
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

		if _, exists := r.Header["Authorization"]; exists {
			principal, err := a.requestPrincipal(r)
			if err != nil {
				writeAuthError(w, err)
				return
			}

			auditActor(r, principal)

			if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || r.URL.Path == "/api/v1/tokens" || strings.HasPrefix(r.URL.Path, "/api/v1/tokens/") {
				writeError(w, repository.ErrAccessDenied)
				return
			}

			ctx := service.WithActor(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, principalKey{}, principal)))

			return
		}

		public := (r.Method == http.MethodGet || r.Method == http.MethodHead) && (r.URL.Path == "/api/v1/auth/session" || r.URL.Path == "/api/v1/auth/oidc/start" || r.URL.Path == "/api/v1/auth/oidc/callback") || r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/setup")
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
			auditActor(r, service.PrincipalFromSession(session))
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid CSRF token; reload and try again"})

			return
		}

		actor := service.PrincipalFromSession(session)
		auditActor(r, actor)

		principal := actor

		if r.URL.Path != "/api/v1/auth/view-as" && r.URL.Path != "/api/v1/auth/logout" {
			var preview *viewAsState

			principal, preview, err = a.browserPrincipal(r, session)
			if err == nil && preview != nil && !previewRequest(r) {
				auditDetail(r, "auth.view_as.read_only", fmt.Sprintf("user:%d", principal.UserID))
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "View as is read-only. Return to Admin to make changes."})

				return
			}
		}

		if err != nil {
			writeError(w, err)
			return
		}

		ctx := service.WithActor(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, principalKey{}, principal)))
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
		a.writeLoginState(w, r, session)
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

	localLogin, err := a.auth.LocalLoginEnabled(r.Context())
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, authState{SetupRequired: setup, LocalLogin: localLogin, OIDCEnabled: a.oidc != nil})
}

func (a *API) writeLoginState(w http.ResponseWriter, r *http.Request, session database.UserSession) {
	state := sessionState(session)

	principal, preview, err := a.browserPrincipal(r, session)
	if previewUnavailable(err) {
		a.setViewAsCookie(w, r, session, 0)

		state.ViewAsEnded = true
		principal, err = a.access.EffectivePrincipal(r.Context(), service.PrincipalFromSession(session))
	}

	if err != nil {
		writeError(w, err)
		return
	}

	state.Principal = &principal
	state.ViewAs = preview

	state.LocalLogin, err = a.auth.LocalLoginEnabled(r.Context())
	if err != nil {
		writeAuthError(w, err)
		return
	}

	state.OIDCEnabled = a.oidc != nil

	writeJSON(w, http.StatusOK, state)
}

func (a *API) authAttempt(w http.ResponseWriter, r *http.Request, setup bool) {
	localLogin, err := a.auth.LocalLoginEnabled(r.Context())
	if err != nil {
		writeAuthError(w, err)
		return
	}

	if !localLogin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "local login is disabled; sign in with OpenID Connect"})
		return
	}

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
	auditActor(r, service.PrincipalFromSession(session))
	a.writeLoginState(w, r, session)
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
