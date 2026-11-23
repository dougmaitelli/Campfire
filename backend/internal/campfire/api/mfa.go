package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"github.com/skip2/go-qrcode"
)

func (a *API) mfaStatus(w http.ResponseWriter, r *http.Request) {
	session, _, err := a.auth.Session(r.Context(), browserToken(r))
	if err != nil {
		writeAuthError(w, err)
		return
	}
	status, err := a.auth.MFAStatus(r.Context(), session)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func writeMFAError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrAuthentication) || errors.Is(err, repository.ErrMFARequired) || errors.Is(err, service.ErrMFACode) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "verification failed; check your credentials and use a fresh authenticator code or an unused recovery code"})
		return
	}
	writeAuthError(w, err)
}

func (a *API) beginMFA(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeRequest(w, r, &input, 4096) {
		return
	}
	enrollment, err := a.auth.BeginMFA(r.Context(), browserToken(r), input.Name)
	if err != nil {
		writeMFAError(w, err)
		return
	}
	png, err := qrcode.Encode(enrollment.URI, qrcode.Medium, 256)
	if err != nil {
		writeError(w, err)
		return
	}
	auditDetail(r, "auth.mfa.enroll.begin", "")
	writeJSON(w, http.StatusOK, struct {
		service.MFAEnrollment
		QRCode string `json:"qr_code"`
	}{enrollment, "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}

func (a *API) removeMFADevice(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}
	if err := a.auth.RemoveMFADevice(r.Context(), browserToken(r), r.PathValue("deviceID"), input.Code); err != nil {
		writeMFAError(w, err)
		return
	}
	auditDetail(r, "auth.mfa.device.remove", "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) confirmMFA(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code string `json:"code"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}
	codes, err := a.auth.ConfirmMFA(r.Context(), browserToken(r), input.Code)
	if err != nil {
		writeMFAError(w, err)
		return
	}
	auditDetail(r, "auth.mfa.enroll.complete", "")
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (a *API) disableMFA(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decodeRequest(w, r, &input, 4096) {
		return
	}
	if err := a.auth.DisableMFA(r.Context(), browserToken(r), input.Password, input.Code); err != nil {
		writeMFAError(w, err)
		return
	}
	auditDetail(r, "auth.mfa.disable", "")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) adminResetMFA(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	if !actor.Admin || actor.APITokenID != 0 {
		writeError(w, repository.ErrAccessDenied)
		return
	}
	id, ok := pathNumber(w, r, "userID")
	if !ok {
		return
	}
	if err := a.auth.AdminResetMFA(r.Context(), browserToken(r), id); err != nil {
		writeMFAError(w, err)
		return
	}
	auditDetail(r, "user.mfa.reset", "user:"+strconv.FormatInt(id, 10))
	w.WriteHeader(http.StatusNoContent)
}
