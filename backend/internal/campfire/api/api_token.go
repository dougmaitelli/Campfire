package api

import (
	"net/http"
	"strconv"

	"campfire/internal/campfire/service"
	accessperm "campfire/internal/shared/permissions"
)

func (a *API) listAPITokens(w http.ResponseWriter, r *http.Request) {
	items, err := a.auth.APITokens(r.Context(), currentUser(r))
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tokens": items, "scopes": service.APITokenScopes, "permissions": accessperm.Catalog()})
}

func (a *API) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name          string   `json:"name"`
		ExpiresInDays int      `json:"expires_in_days"`
		Scopes        []string `json:"scopes"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	item, secret, err := a.auth.CreateAPIToken(r.Context(), currentUser(r), input.Name, input.ExpiresInDays, input.Scopes)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	auditDetail(r, "token.create", "token:"+strconv.FormatInt(item.ID, 10))
	writeJSON(w, http.StatusCreated, map[string]any{"token": item, "secret": secret})
}

func (a *API) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "tokenID")
	if !ok {
		return
	}

	if err := a.auth.RevokeAPIToken(r.Context(), currentUser(r), id); err != nil {
		writeAuthError(w, err)
		return
	}

	auditDetail(r, "token.revoke", "token:"+strconv.FormatInt(id, 10))
	w.WriteHeader(http.StatusNoContent)
}
