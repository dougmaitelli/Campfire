package api

import (
	"net/http"
	"strconv"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func pathNumber(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, repository.ErrAccessInput)
		return 0, false
	}

	return id, true
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.access.Users(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, users)
}

type userInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	RoleID   int64  `json:"role_id"`
	Disabled bool   `json:"disabled"`
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var input userInput
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	user, err := a.access.CreateUser(r.Context(), input.Username, input.Password, input.RoleID, input.Disabled, input.Name, input.Email)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	auditDetail(r, "user.create", "user:"+strconv.FormatInt(user.ID, 10))
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "userID")
	if !ok {
		return
	}

	var input struct {
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		RoleID   int64   `json:"role_id"`
		Disabled bool    `json:"disabled"`
		Password string  `json:"password"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	user, err := a.access.UpdateUser(r.Context(), id, input.RoleID, input.Disabled, input.Password, input.Name, input.Email)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	auditDetail(r, "user.update", "user:"+strconv.FormatInt(user.ID, 10))
	writeJSON(w, http.StatusOK, user)
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "userID")
	if !ok {
		return
	}

	if err := a.access.DeleteUser(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := a.access.Roles(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, roles)
}

func (a *API) saveRole(w http.ResponseWriter, r *http.Request, id int64) {
	var input struct {
		Name        string   `json:"name"`
		Scope       string   `json:"scope"`
		Permissions []string `json:"permissions"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	role, err := a.access.SaveRole(r.Context(), id, input.Name, input.Scope, input.Permissions)
	if err != nil {
		writeError(w, err)
		return
	}

	status := http.StatusOK
	if id == 0 {
		status = http.StatusCreated
	}

	auditDetail(r, "role.save", "role:"+strconv.FormatInt(role.ID, 10))
	writeJSON(w, status, role)
}

func (a *API) createRole(w http.ResponseWriter, r *http.Request) { a.saveRole(w, r, 0) }
func (a *API) updateRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "roleID")
	if ok {
		a.saveRole(w, r, id)
	}
}
func (a *API) deleteRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "roleID")
	if !ok {
		return
	}

	if err := a.access.DeleteRole(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type serverMember struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	RoleID   int64  `json:"role_id"`
	Role     string `json:"role"`
}

func (a *API) serverAccess(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.servers.Get(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}

	owner, members, err := a.access.ServerAccess(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	views := make([]serverMember, 0, len(members))
	for _, member := range members {
		views = append(views, serverMember{member.UserID, member.User.Username, member.RoleID, member.Role.Name})
	}

	allowed, err := a.assignableServerRoles(r)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		Owner   *database.User     `json:"owner"`
		Members []serverMember     `json:"members"`
		Roles   []service.RoleView `json:"roles"`
	}{owner, views, allowed})
}

func (a *API) assignableServerRoles(r *http.Request) ([]service.RoleView, error) {
	roles, err := a.access.Roles(r.Context())
	if err != nil {
		return nil, err
	}

	allowed := []service.RoleView{}

	for _, role := range roles {
		if role.Scope == "server" && role.ID != database.OwnerRole && (currentUser(r).Admin || role.ID == database.OperatorRole) {
			allowed = append(allowed, role)
		}
	}

	return allowed, nil
}

func (a *API) serverAccessOptions(w http.ResponseWriter, r *http.Request) {
	roles, err := a.assignableServerRoles(r)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"owner": nil, "members": []serverMember{}, "roles": roles})
}

func (a *API) shareServer(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		RoleID   int64  `json:"role_id"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	if input.RoleID == 0 {
		input.RoleID = database.OperatorRole
	}

	if err := a.access.Share(r.Context(), currentUser(r), r.PathValue("id"), input.Username, input.RoleID); err != nil {
		writeError(w, err)
		return
	}

	a.serverAccess(w, r)
}

func (a *API) unshareServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "userID")
	if !ok {
		return
	}

	if err := a.access.RemoveMember(r.Context(), r.PathValue("id"), id); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) transferServer(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID int64 `json:"user_id"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if input.UserID <= 0 {
		writeError(w, repository.ErrAccessInput)
		return
	}

	if err := a.access.TransferOwner(r.Context(), r.PathValue("id"), input.UserID); err != nil {
		writeError(w, err)
		return
	}

	a.serverAccess(w, r)
}

func (a *API) sparkRoles(w http.ResponseWriter, r *http.Request) {
	if _, err := a.sparks.Get(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}

	roles, err := a.access.SparkRoles(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"role_ids": roles})
}
func (a *API) setSparkRoles(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleIDs []int64 `json:"role_ids"`
	}
	if !decodeRequest(w, r, &input, 8192) {
		return
	}

	if err := a.access.SetSparkRoles(r.Context(), r.PathValue("id"), input.RoleIDs); err != nil {
		writeError(w, err)
		return
	}

	a.sparkRoles(w, r)
}

type editorOptions struct {
	Sparks      []model.Spark `json:"sparks"`
	Embers      []model.Ember `json:"embers"`
	S3Available bool          `json:"s3_available"`
}

func (a *API) provisioning(w http.ResponseWriter, r *http.Request) {
	sparks, err := a.sparks.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	allowed := []model.Spark{}

	for _, spark := range sparks {
		ok, err := a.access.CanProvision(r.Context(), currentUser(r), spark.ID)
		if err != nil {
			writeError(w, err)
			return
		}

		if ok && !spark.Disabled {
			allowed = append(allowed, spark)
		}
	}

	embers, err := a.embers.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, editorOptions{allowed, embers, a.deployment != nil && a.deployment.Backups != nil && a.deployment.Backups.Remote != nil})
}

func (a *API) serverEditor(w http.ResponseWriter, r *http.Request) {
	server, err := a.servers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}

	spark, err := a.sparks.Get(r.Context(), server.SparkID)
	if err != nil {
		writeError(w, err)
		return
	}

	ember, err := a.embers.Get(r.Context(), server.EmberID)
	if err != nil {
		writeError(w, err)
		return
	}

	embers := []model.Ember{ember}
	if currentUser(r).Allows("servers.manage_all") {
		embers, err = a.embers.List(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, editorOptions{[]model.Spark{spark}, embers, a.deployment != nil && a.deployment.Backups != nil && a.deployment.Backups.Remote != nil})
}
