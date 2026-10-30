package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
)

const viewAsCookie = "campfire_view_as"

// viewAsState identifies the real administrator while Principal describes the previewed user.
type viewAsState struct {
	AdminUserID   int64  `json:"admin_user_id"`
	AdminUsername string `json:"admin_username"`
	AdminName     string `json:"admin_name"`
}

func viewAsSignature(session database.UserSession, value string) []byte {
	mac := hmac.New(sha256.New, []byte(session.Hash))
	_, _ = mac.Write([]byte(value))

	return mac.Sum(nil)
}

func viewAsTarget(r *http.Request, session database.UserSession) int64 {
	cookie, err := r.Cookie(viewAsCookie)
	if err != nil {
		return 0
	}

	value, signature, ok := strings.Cut(cookie.Value, ".")

	decoded, err := hex.DecodeString(signature)
	if !ok || err != nil || !hmac.Equal(decoded, viewAsSignature(session, value)) {
		return 0
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}

	return id
}

func (a *API) setViewAsCookie(w http.ResponseWriter, r *http.Request, session database.UserSession, id int64) {
	cookie := &http.Cookie{Name: viewAsCookie, Path: "/api", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(a.origin, "https://"), SameSite: http.SameSiteStrictMode, Expires: time.Unix(session.ExpiresAt, 0)}
	if id == 0 {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
	} else {
		value := strconv.FormatInt(id, 10)
		cookie.Value = value + "." + hex.EncodeToString(viewAsSignature(session, value))
	}

	http.SetCookie(w, cookie)
}

func (a *API) browserPrincipal(r *http.Request, session database.UserSession) (service.Principal, *viewAsState, error) {
	actor := service.PrincipalFromSession(session)

	id := viewAsTarget(r, session)
	if !actor.Admin || id == 0 {
		principal, err := a.access.EffectivePrincipal(r.Context(), actor)
		return principal, nil, err
	}

	principal, err := a.access.ViewAsPrincipal(r.Context(), actor, id)
	if err != nil {
		return service.Principal{}, nil, err
	}

	return principal, &viewAsState{actor.UserID, actor.Username, actor.Name}, nil
}

func (a *API) startViewAs(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	if !actor.Admin || actor.RoleID != database.AdminRole || actor.APITokenID != 0 {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	var input struct {
		UserID int64 `json:"user_id"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if _, err := a.access.ViewAsPrincipal(r.Context(), actor, input.UserID); err != nil {
		writeError(w, err)
		return
	}

	session, _, err := a.auth.Session(r.Context(), browserToken(r))
	if err != nil {
		writeAuthError(w, err)
		return
	}

	auditDetail(r, "auth.view_as.start", fmt.Sprintf("user:%d", input.UserID))
	a.setViewAsCookie(w, r, session, input.UserID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) stopViewAs(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	if !actor.Admin || actor.RoleID != database.AdminRole || actor.APITokenID != 0 {
		writeError(w, repository.ErrAccessDenied)
		return
	}

	session, _, err := a.auth.Session(r.Context(), browserToken(r))
	if err != nil {
		writeAuthError(w, err)
		return
	}

	auditDetail(r, "auth.view_as.stop", fmt.Sprintf("user:%d", viewAsTarget(r, session)))
	a.setViewAsCookie(w, r, session, 0)
	w.WriteHeader(http.StatusNoContent)
}

// previewRequest permits reads, including file reads transported as POSTs.
// Normal permission checks still run after this check.
func previewRequest(r *http.Request) bool {
	if r.URL.Path == "/api/v1/tokens" || strings.HasPrefix(r.URL.Path, "/api/v1/tokens/") {
		return false
	}

	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}

	if r.Method != http.MethodPost {
		return false
	}

	if r.URL.Path == "/api/v1/schedules/preview" {
		return true
	}

	if !strings.HasPrefix(r.URL.Path, "/api/v1/servers/") || !strings.HasSuffix(r.URL.Path, "/files") {
		return false
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return false
	}

	r.Body = io.NopCloser(bytes.NewReader(data))

	var input struct {
		Operation string `json:"operation"`
	}
	if json.Unmarshal(data, &input) != nil {
		return false
	}

	switch input.Operation {
	case "list", "stat", "read", "read.chunk":
		return true
	default:
		return false
	}
}

func previewUnavailable(err error) bool {
	return errors.Is(err, repository.ErrAccessNotFound) || errors.Is(err, repository.ErrAccessInput)
}
