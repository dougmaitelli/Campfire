package api

import "net/http"

func (a *API) authSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := a.auth.Sessions(r.Context(), browserToken(r))
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (a *API) authRevokeSession(w http.ResponseWriter, r *http.Request) {
	current, err := a.auth.RevokeSession(r.Context(), browserToken(r), r.PathValue("id"))
	if err != nil {
		writeAuthError(w, err)
		return
	}
	auditDetail(r, "auth.session.revoke", "session:"+r.PathValue("id"))
	if current {
		a.setSessionCookie(w, r, "", 0)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) authRevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.RevokeOtherSessions(r.Context(), browserToken(r)); err != nil {
		writeAuthError(w, err)
		return
	}
	auditDetail(r, "auth.sessions.revoke_others", "")
	w.WriteHeader(http.StatusNoContent)
}
