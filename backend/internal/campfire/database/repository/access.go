package repository

import (
	"context"
	"errors"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
)

// Account and access errors distinguish invalid requests from permission failures.
var (
	ErrAccessDenied   = errors.New("you do not have permission for this action")
	ErrAccessInput    = errors.New("invalid account, role or server assignment")
	ErrAccessConflict = errors.New("account or role is already in use")
	ErrLastAdmin      = errors.New("at least one enabled administrator is required")
	ErrAccessNotFound = errors.New("account or role not found")
)

// AccessRepository persists account roles, ownership, memberships and provisioning grants.
type AccessRepository struct {
	db     *gorm.DB
	issuer string
}

// NewAccessRepository constructs access storage on the panel database.
func NewAccessRepository(db *gorm.DB) *AccessRepository { return &AccessRepository{db: db} }

// WithRequiredOIDC preserves an administrator who can use the active login provider.
func (r *AccessRepository) WithRequiredOIDC(issuer string) *AccessRepository {
	r.issuer = issuer
	return r
}

func accessError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAccessNotFound
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrForeignKeyViolated) {
		return ErrAccessConflict
	}

	return err
}

// Roles returns the permission definitions, including built-ins.
func (r *AccessRepository) Roles(ctx context.Context) ([]database.Role, error) {
	roles := []database.Role{}
	err := r.db.WithContext(ctx).Preload("Permissions").Order("id").Find(&roles).Error

	return roles, err
}

// Role loads the current permission set.
func (r *AccessRepository) Role(ctx context.Context, id int64) (database.Role, error) {
	var role database.Role

	err := r.db.WithContext(ctx).Preload("Permissions").Take(&role, id).Error

	return role, accessError(err)
}

// SaveRole replaces a custom role's permissions atomically.
func (r *AccessRepository) SaveRole(ctx context.Context, role *database.Role) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if role.ID != 0 {
			var current database.Role
			if err := tx.Take(&current, role.ID).Error; err != nil {
				return err
			}

			if current.Builtin || current.Scope != role.Scope {
				return ErrAccessDenied
			}

			if err := tx.Model(&current).Update("name", role.Name).Error; err != nil {
				return err
			}

			if err := tx.Where("role_id = ?", role.ID).Delete(&database.RolePermission{}).Error; err != nil {
				return err
			}
		} else if err := tx.Omit("Permissions").Create(role).Error; err != nil {
			return err
		}

		for _, permission := range role.Permissions {
			permission.RoleID = role.ID
			if err := tx.Create(&permission).Error; err != nil {
				return err
			}
		}

		return nil
	}))
}

// DeleteRole rejects built-in or referenced roles.
func (r *AccessRepository) DeleteRole(ctx context.Context, id int64) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var role database.Role
		if err := tx.Take(&role, id).Error; err != nil {
			return err
		}

		if role.Builtin {
			return ErrAccessDenied
		}

		var users, members int64
		if err := tx.Unscoped().Model(&database.User{}).Where("role_id = ?", id).Count(&users).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.ServerAccessGrant{}).Where("access_role_id = ?", id).Count(&members).Error; err != nil {
			return err
		}

		if users != 0 || members != 0 {
			return ErrAccessConflict
		}

		return tx.Delete(&role).Error
	}))
}

// Users returns public account metadata; password hashes are excluded by JSON tags.
func (r *AccessRepository) Users(ctx context.Context) ([]database.User, error) {
	users := []database.User{}
	err := r.db.WithContext(ctx).Preload("Role").Order("username").Find(&users).Error

	return users, err
}

// User loads an account and role.
func (r *AccessRepository) User(ctx context.Context, id int64) (database.User, error) {
	var user database.User

	err := r.db.WithContext(ctx).Preload("Role").Take(&user, id).Error

	return user, accessError(err)
}

// CreateUser adds an account after verifying the role scope.
func (r *AccessRepository) CreateUser(ctx context.Context, user *database.User) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var role database.Role
		if err := tx.Where("id = ? AND scope = 'platform'", user.RoleID).Take(&role).Error; err != nil {
			return ErrAccessInput
		}

		return tx.Omit("Role").Create(user).Error
	}))
}

func (r *AccessRepository) keepAdmin(tx *gorm.DB, user database.User, roleID int64, disabled bool) error {
	if user.RoleID != database.AdminRole || user.Disabled || roleID == database.AdminRole && !disabled {
		return nil
	}

	var count int64

	query := tx.Model(&database.User{}).Where("role_id = ? AND disabled = false AND id != ?", database.AdminRole, user.ID)
	if r.issuer != "" {
		ready, err := NewAuthRepository(tx).HasOIDCAdmin(tx.Statement.Context, r.issuer)
		if err != nil {
			return err
		}

		if ready {
			query = query.Where("id IN (?)", tx.Model(&database.OIDCIdentity{}).Select("user_id").Where("issuer = ?", r.issuer))
		}
	}

	if err := query.Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		return ErrLastAdmin
	}

	return nil
}

// UpdateUser revokes that account's sessions when its credentials or role change.
func (r *AccessRepository) UpdateUser(ctx context.Context, id int64, roleID int64, disabled bool, password string, name, email *string) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Take(&user, id).Error; err != nil {
			return err
		}

		var role database.Role
		if err := tx.Where("id = ? AND scope = 'platform'", roleID).Take(&role).Error; err != nil {
			return ErrAccessInput
		}

		if err := r.keepAdmin(tx, user, roleID, disabled); err != nil {
			return err
		}

		updates := map[string]any{}

		if name != nil {
			if user.OIDCNameManaged && *name != user.Name {
				return ErrAccessInput
			}

			updates["name"] = *name
		}

		if email != nil {
			if user.OIDCEmailManaged && *email != user.Email {
				return ErrAccessInput
			}

			updates["email"] = *email
		}

		if user.RoleID == roleID && user.Disabled == disabled && password == "" {
			if len(updates) == 0 {
				return nil
			}

			return tx.Model(&user).Updates(updates).Error
		}

		updates["role_id"], updates["disabled"], updates["credential_version"] = roleID, disabled, gorm.Expr("credential_version + 1")

		if password != "" {
			updates["password_hash"] = password
		}

		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}

		return tx.Where("user_id = ?", id).Delete(&database.UserSession{}).Error
	}))
}

// DeleteUser preserves owned servers and prevents losing the last enabled admin.
func (r *AccessRepository) DeleteUser(ctx context.Context, id int64) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Take(&user, id).Error; err != nil {
			return err
		}

		if err := r.keepAdmin(tx, user, 0, true); err != nil {
			return err
		}

		var owned int64
		if err := tx.Model(&database.Server{}).Where("owner_user_id = ?", id).Count(&owned).Error; err != nil {
			return err
		}

		if owned != 0 {
			return ErrAccessConflict
		}

		if err := tx.Model(&user).Updates(map[string]any{"disabled": true, "password_hash": "", "credential_version": gorm.Expr("credential_version + 1")}).Error; err != nil {
			return err
		}

		for _, record := range []any{&database.UserSession{}, &database.APIToken{}, &database.ServerAccessGrant{}, &database.SparkAccessGrant{}, &database.NotificationRecipient{}} {
			if err := tx.Where("user_id = ?", id).Delete(record).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&database.Schedule{}).Where("run_as_user_id = ?", id).Update("enabled", false).Error; err != nil {
			return err
		}

		return tx.Delete(&user).Error
	}))
}

// Owner returns the account referenced directly by the server.
func (r *AccessRepository) Owner(ctx context.Context, serverID string) (*database.User, error) {
	var user database.User

	err := r.db.WithContext(ctx).Preload("Role").Joins("JOIN servers ON servers.owner_user_id = users.id").Where("servers.id = ?", serverID).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return &user, err
}

// Members lists explicit server grants.
func (r *AccessRepository) Members(ctx context.Context, serverID string) ([]database.ServerAccessGrant, error) {
	items := []database.ServerAccessGrant{}
	err := r.db.WithContext(ctx).Preload("User").Preload("Role").Preload("AccessRole.Permissions").Where("server_id = ?", serverID).Order("id").Find(&items).Error

	return items, err
}

// MemberRole combines direct user grants and grants to the user's current account role.
func (r *AccessRepository) MemberRole(ctx context.Context, serverID string, userID int64) (database.Role, error) {
	permissions := []database.RolePermission{}
	err := r.db.WithContext(ctx).Model(&database.RolePermission{}).
		Joins("JOIN server_access_grants ON server_access_grants.access_role_id = role_permissions.role_id").
		Joins("JOIN users ON users.id = ? AND users.deleted_at IS NULL AND users.disabled = false AND (server_access_grants.user_id = users.id OR server_access_grants.role_id = users.role_id)", userID).
		Where("server_access_grants.server_id = ?", serverID).Distinct("role_permissions.permission").Find(&permissions).Error

	return database.Role{Permissions: permissions}, err
}

func validateGrantSubject(tx *gorm.DB, userID, roleID *int64) error {
	if (userID == nil) == (roleID == nil) {
		return ErrAccessInput
	}

	if userID != nil {
		var count int64
		if err := tx.Model(&database.User{}).Where("id = ? AND disabled = false", *userID).Count(&count).Error; err != nil {
			return err
		}

		if count != 1 {
			return ErrAccessInput
		}
	} else {
		var count int64
		if err := tx.Model(&database.Role{}).Where("id = ? AND scope = 'platform'", *roleID).Count(&count).Error; err != nil {
			return err
		}

		if count != 1 {
			return ErrAccessInput
		}
	}

	return nil
}

// SetMember upserts one subject's server access level without changing ownership.
func (r *AccessRepository) SetMember(ctx context.Context, grant database.ServerAccessGrant) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := validateGrantSubject(tx, grant.UserID, grant.RoleID); err != nil {
			return err
		}

		var level database.Role
		if err := tx.Where("id = ? AND scope = 'server'", grant.AccessRoleID).Take(&level).Error; err != nil || level.ID == database.OwnerRole {
			return ErrAccessInput
		}

		if grant.UserID != nil {
			var count int64
			if err := tx.Model(&database.Server{}).Where("id = ? AND owner_user_id = ?", grant.ServerID, *grant.UserID).Count(&count).Error; err != nil {
				return err
			}

			if count > 0 {
				return ErrAccessInput
			}
		}

		query := tx.Where("server_id = ?", grant.ServerID)
		if grant.UserID != nil {
			query = query.Where("user_id = ?", *grant.UserID)
		} else {
			query = query.Where("role_id = ?", *grant.RoleID)
		}

		var current database.ServerAccessGrant
		if err := query.Take(&current).Error; err == nil {
			return tx.Model(&current).Update("access_role_id", grant.AccessRoleID).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return tx.Omit("Server", "User", "Role", "AccessRole").Create(&grant).Error
	}))
}

// RemoveMember removes a legacy direct user grant.
func (r *AccessRepository) RemoveMember(ctx context.Context, id string, userID int64) error {
	return r.db.WithContext(ctx).Where("server_id = ? AND user_id = ?", id, userID).Delete(&database.ServerAccessGrant{}).Error
}

// RemoveServerGrant removes one server access grant.
func (r *AccessRepository) RemoveServerGrant(ctx context.Context, id string, grantID int64) error {
	return r.db.WithContext(ctx).Where("server_id = ? AND id = ?", id, grantID).Delete(&database.ServerAccessGrant{}).Error
}

// TransferOwner changes ownership and clears the new owner's redundant grant.
func (r *AccessRepository) TransferOwner(ctx context.Context, serverID string, userID int64) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var user database.User
		if err := tx.Where("id = ? AND disabled = false", userID).Take(&user).Error; err != nil {
			return ErrAccessInput
		}

		result := tx.Model(&database.Server{}).Where("id = ?", serverID).Update("owner_user_id", userID)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return ErrAccessNotFound
		}

		return tx.Where("server_id = ? AND user_id = ?", serverID, userID).Delete(&database.ServerAccessGrant{}).Error
	}))
}

// SparkRoles is retained for older API clients; user grants are managed separately.
func (r *AccessRepository) SparkRoles(ctx context.Context, sparkID string) ([]int64, error) {
	ids := []int64{}
	err := r.db.WithContext(ctx).Model(&database.SparkAccessGrant{}).Where("spark_id = ? AND role_id IS NOT NULL", sparkID).Order("role_id").Pluck("role_id", &ids).Error

	return ids, err
}

// SetSparkRoles replaces account role grants while retaining direct user grants.
func (r *AccessRepository) SetSparkRoles(ctx context.Context, sparkID string, ids []int64) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var spark database.Spark
		if err := tx.Take(&spark, "id = ?", sparkID).Error; err != nil {
			return err
		}

		if err := tx.Where("spark_id = ? AND role_id IS NOT NULL", sparkID).Delete(&database.SparkAccessGrant{}).Error; err != nil {
			return err
		}

		seen := map[int64]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}

			seen[id] = true
			if err := validateGrantSubject(tx, nil, &id); err != nil {
				return err
			}

			if id == database.AdminRole {
				return ErrAccessInput
			}

			if err := tx.Omit("Spark", "User", "Role", "AccessRole").Create(&database.SparkAccessGrant{SparkID: sparkID, RoleID: &id, AccessRoleID: database.CreatorRole}).Error; err != nil {
				return err
			}
		}

		return nil
	}))
}

// SparkGrants lists explicit Spark provisioning grants.
func (r *AccessRepository) SparkGrants(ctx context.Context, id string) ([]database.SparkAccessGrant, error) {
	items := []database.SparkAccessGrant{}
	err := r.db.WithContext(ctx).Preload("User").Preload("Role").Preload("AccessRole.Permissions").Where("spark_id = ?", id).Order("id").Find(&items).Error

	return items, err
}

// SetSparkGrant grants server creation on a Spark.
func (r *AccessRepository) SetSparkGrant(ctx context.Context, grant database.SparkAccessGrant) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := validateGrantSubject(tx, grant.UserID, grant.RoleID); err != nil {
			return err
		}

		grant.AccessRoleID = database.CreatorRole

		query := tx.Where("spark_id = ?", grant.SparkID)
		if grant.UserID != nil {
			query = query.Where("user_id = ?", *grant.UserID)
		} else {
			query = query.Where("role_id = ?", *grant.RoleID)
		}

		var current database.SparkAccessGrant
		if err := query.Take(&current).Error; err == nil {
			return tx.Model(&current).Update("access_role_id", grant.AccessRoleID).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return tx.Omit("Spark", "User", "Role", "AccessRole").Create(&grant).Error
	}))
}

// RemoveSparkGrant revokes one Spark provisioning grant.
func (r *AccessRepository) RemoveSparkGrant(ctx context.Context, id string, grantID int64) error {
	return r.db.WithContext(ctx).Where("spark_id = ? AND id = ?", id, grantID).Delete(&database.SparkAccessGrant{}).Error
}

// HasSparkGrant checks current assignments; an empty Spark ID checks any eligible Spark.
func (r *AccessRepository) HasSparkGrant(ctx context.Context, userID, roleID int64, sparkID string) (bool, error) {
	var count int64

	query := r.db.WithContext(ctx).Model(&database.SparkAccessGrant{}).Joins("JOIN sparks ON sparks.id = spark_access_grants.spark_id").
		Joins("JOIN users ON users.id = ? AND users.deleted_at IS NULL AND users.disabled = false", userID).
		Where("sparks.disabled = false AND (spark_access_grants.user_id = ? OR spark_access_grants.role_id = ?)", userID, roleID)
	if sparkID != "" {
		query = query.Where("spark_access_grants.spark_id = ?", sparkID)
	}

	err := query.Count(&count).Error

	return count > 0, err
}
