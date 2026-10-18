package api

import (
	"net/http"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
)

type accessGrantView struct {
	ID           int64  `json:"id"`
	UserID       *int64 `json:"user_id"`
	Username     string `json:"username"`
	RoleID       *int64 `json:"role_id"`
	RoleName     string `json:"role_name"`
	AccessRoleID int64  `json:"access_role_id"`
	AccessRole   string `json:"access_role"`
}

func grantView(id int64, userID *int64, user *database.User, roleID *int64, role *database.Role, accessRole database.Role) accessGrantView {
	view := accessGrantView{ID: id, UserID: userID, RoleID: roleID, AccessRoleID: accessRole.ID, AccessRole: accessRole.Name}
	if user != nil {
		view.Username = user.Username
	}

	if role != nil {
		view.RoleName = role.Name
	}

	return view
}

func serverGrantViews(items []database.ServerAccessGrant) []accessGrantView {
	views := make([]accessGrantView, 0, len(items))
	for _, item := range items {
		views = append(views, grantView(item.ID, item.UserID, item.User, item.RoleID, item.Role, item.AccessRole))
	}

	return views
}

func (a *API) subjectRoles(r *http.Request) ([]service.RoleView, error) {
	roles, err := a.access.Roles(r.Context())
	if err != nil {
		return nil, err
	}

	items := []service.RoleView{}

	for _, role := range roles {
		if role.Scope == "platform" && role.ID != database.AdminRole {
			items = append(items, role)
		}
	}

	return items, nil
}

func (a *API) sparkAccess(w http.ResponseWriter, r *http.Request) {
	if _, err := a.sparks.Get(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}

	items, err := a.access.SparkGrants(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	views := make([]accessGrantView, 0, len(items))
	for _, item := range items {
		views = append(views, grantView(item.ID, item.UserID, item.User, item.RoleID, item.Role, item.AccessRole))
	}

	subjects, err := a.subjectRoles(r)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"grants": views, "subject_roles": subjects})
}

func (a *API) grantSparkAccess(w http.ResponseWriter, r *http.Request) {
	var input service.AccessGrantInput
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	if err := a.access.GrantSpark(r.Context(), r.PathValue("id"), input); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) grantServerAccess(w http.ResponseWriter, r *http.Request) {
	var input service.AccessGrantInput
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	if err := a.access.GrantServer(r.Context(), currentUser(r), r.PathValue("id"), input); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) removeServerAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "grantID")
	if !ok {
		return
	}

	if err := a.access.RemoveServerGrant(r.Context(), r.PathValue("id"), id); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) removeSparkAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "grantID")
	if !ok {
		return
	}

	if err := a.access.RemoveSparkGrant(r.Context(), r.PathValue("id"), id); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
