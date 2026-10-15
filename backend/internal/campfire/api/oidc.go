package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
)

const oidcCookie = "campfire_oidc"

func (a *API) oidcFlowCookie(w http.ResponseWriter, r *http.Request, value string) {
	cookie := &http.Cookie{Name: oidcCookie, Value: value, Path: "/api/v1/auth/oidc", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(a.origin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 300}
	if value == "" {
		cookie.MaxAge = -1
	}

	http.SetCookie(w, cookie)
}

func (a *API) startOIDC(w http.ResponseWriter, r *http.Request, link bool) {
	if a.oidc == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "OpenID Connect is not enabled"})
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

	var linkToken string
	if link {
		linkToken = browserToken(r)
	}

	target, cookie, err := a.oidc.Start(r.Context(), linkToken)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	a.oidcFlowCookie(w, r, cookie)

	if link {
		writeJSON(w, http.StatusOK, map[string]string{"url": target})
		return
	}

	http.Redirect(w, r, target, http.StatusFound)
}

func (a *API) oidcStart(w http.ResponseWriter, r *http.Request) {
	a.startOIDC(w, r, false)
}

func (a *API) oidcLink(w http.ResponseWriter, r *http.Request) {
	a.startOIDC(w, r, true)
}

func (a *API) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "OpenID Connect is not enabled"})
		return
	}

	cookie, err := r.Cookie(oidcCookie)
	a.oidcFlowCookie(w, r, "")

	if err != nil {
		if event, ok := r.Context().Value(auditKey{}).(*database.AuditEvent); ok {
			event.Outcome = "failed"
		}

		http.Redirect(w, r, "/?oidc_error=1", http.StatusSeeOther)

		return
	}

	ctx := r.Context()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	session, token, err := a.oidc.Complete(ctx, cookie.Value, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
	if err != nil {
		if event, ok := r.Context().Value(auditKey{}).(*database.AuditEvent); ok {
			event.Outcome = "failed"
		}

		http.Redirect(w, r, "/?oidc_error=1", http.StatusSeeOther)

		return
	}

	a.setSessionCookie(w, r, token, session.ExpiresAt)
	auditActor(r, service.PrincipalFromSession(session))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
