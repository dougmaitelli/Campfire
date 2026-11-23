package api

import (
	"net/http"

	"campfire/internal/campfire/database"
)

func (a *API) readProfile(w http.ResponseWriter, r *http.Request) {
	session, _, err := a.auth.Session(r.Context(), browserToken(r))
	if err != nil {
		writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		User          database.User `json:"user"`
		LocalPassword bool          `json:"local_password"`
	}{session.User, session.Provider == "" && session.User.PasswordHash != ""})
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name            string `json:"name"`
		Email           string `json:"email"`
		Password        string `json:"password"`
		CurrentPassword string `json:"current_password"`
		Code            string `json:"code"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}
	user, err := a.auth.UpdateProfile(r.Context(), browserToken(r), input.Name, input.Email, input.Password, input.CurrentPassword, input.Code)
	if err != nil {
		writeMFAError(w, err)
		return
	}
	auditDetail(r, "user.profile.update", "")
	writeJSON(w, http.StatusOK, user)
}
