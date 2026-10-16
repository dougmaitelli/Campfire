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
		if err := tx.Model(&database.User{}).Where("role_id = ?", id).Count(&users).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.ServerMember{}).Where("role_id = ?", id).Count(&members).Error; err != nil {
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
		query = query.Where("id IN (?)", tx.Model(&database.OIDCIdentity{}).Select("user_id").Where("issuer = ?", r.issuer))
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
func (r *AccessRepository) UpdateUser(ctx context.Context, id int64, roleID int64, disabled bool, password string) error {
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

		if user.RoleID == roleID && user.Disabled == disabled && password == "" {
			return nil
		}

		updates := map[string]any{"role_id": roleID, "disabled": disabled, "version": gorm.Expr("version + 1")}
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
		if err := tx.Model(&database.ServerOwnership{}).Where("user_id = ?", id).Count(&owned).Error; err != nil {
			return err
		}

		if owned != 0 {
			return ErrAccessConflict
		}

		return tx.Delete(&user).Error
	}))
}

// Owner returns the server's current owner, if one exists before account setup.
func (r *AccessRepository) Owner(ctx context.Context, serverID string) (*database.User, error) {
	var ownership database.ServerOwnership

	err := r.db.WithContext(ctx).Preload("User.Role").Where("server_id = ?", serverID).Take(&ownership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return &ownership.User, err
}

// Members lists server-scoped grants without listing unrelated panel accounts.
func (r *AccessRepository) Members(ctx context.Context, serverID string) ([]database.ServerMember, error) {
	members := []database.ServerMember{}
	err := r.db.WithContext(ctx).Preload("User.Role").Preload("Role.Permissions").Where("server_id = ?", serverID).Order("user_id").Find(&members).Error

	return members, err
}

// MemberRole reads current permissions on every request, so revoked access is immediate.
func (r *AccessRepository) MemberRole(ctx context.Context, serverID string, userID int64) (database.Role, error) {
	var member database.ServerMember

	err := r.db.WithContext(ctx).Preload("Role.Permissions").Where("server_id = ? AND user_id = ?", serverID, userID).Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return database.Role{}, nil
	}

	return member.Role, err
}

// SetMember grants a server role and never changes ownership or platform roles.
func (r *AccessRepository) SetMember(ctx context.Context, member database.ServerMember) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var role database.Role
		if err := tx.Where("id = ? AND scope = 'server'", member.RoleID).Take(&role).Error; err != nil || role.ID == database.OwnerRole {
			return ErrAccessInput
		}

		var ownership database.ServerOwnership
		if err := tx.Where("server_id = ? AND user_id = ?", member.ServerID, member.UserID).Take(&ownership).Error; err == nil {
			return ErrAccessInput
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Where("server_id = ? AND user_id = ?", member.ServerID, member.UserID).Delete(&database.ServerMember{}).Error; err != nil {
			return err
		}

		return tx.Omit("Server", "User", "Role").Create(&member).Error
	}))
}

// RemoveMember revokes only the named collaborator's server access.
func (r *AccessRepository) RemoveMember(ctx context.Context, serverID string, userID int64) error {
	return r.db.WithContext(ctx).Where("server_id = ? AND user_id = ?", serverID, userID).Delete(&database.ServerMember{}).Error
}

// TransferOwner replaces ownership without allowing the old owner to retain implicit access.
func (r *AccessRepository) TransferOwner(ctx context.Context, serverID string, userID int64) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var server database.Server
		if err := tx.Take(&server, "id = ?", serverID).Error; err != nil {
			return err
		}

		var user database.User
		if err := tx.Where("id = ? AND disabled = false", userID).Take(&user).Error; err != nil {
			return ErrAccessInput
		}

		if err := tx.Where("server_id = ?", serverID).Delete(&database.ServerOwnership{}).Error; err != nil {
			return err
		}

		if err := tx.Omit("Server", "User").Create(&database.ServerOwnership{ServerID: serverID, UserID: userID}).Error; err != nil {
			return err
		}

		return tx.Where("server_id = ? AND user_id = ?", serverID, userID).Delete(&database.ServerMember{}).Error
	}))
}

// SparkRoles returns platform roles allowed to provision on the given Spark.
func (r *AccessRepository) SparkRoles(ctx context.Context, sparkID string) ([]int64, error) {
	ids := []int64{}
	err := r.db.WithContext(ctx).Model(&database.SparkRole{}).Where("spark_id = ?", sparkID).Order("role_id").Pluck("role_id", &ids).Error

	return ids, err
}

// SetSparkRoles atomically changes provisioning eligibility, not Spark management rights.
func (r *AccessRepository) SetSparkRoles(ctx context.Context, sparkID string, ids []int64) error {
	return accessError(database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var spark database.Spark
		if err := tx.Take(&spark, "id = ?", sparkID).Error; err != nil {
			return err
		}

		if err := tx.Where("spark_id = ?", sparkID).Delete(&database.SparkRole{}).Error; err != nil {
			return err
		}

		seen := map[int64]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}

			seen[id] = true

			var count int64
			if err := tx.Model(&database.RolePermission{}).Joins("JOIN roles ON roles.id = role_id").Where("role_id = ? AND permission = 'servers.create' AND roles.scope = 'platform'", id).Count(&count).Error; err != nil {
				return err
			}

			if id == database.AdminRole || count != 1 {
				return ErrAccessInput
			}

			if err := tx.Omit("Spark", "Role").Create(&database.SparkRole{SparkID: sparkID, RoleID: id}).Error; err != nil {
				return err
			}
		}

		return nil
	}))
}
