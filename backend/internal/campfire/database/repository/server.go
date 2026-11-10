package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"
	accessperm "campfire/internal/shared/permissions"

	"gorm.io/gorm"
)

// Server repository errors distinguish placement, lookup, and reservation failures.
var (
	ErrServerNotFound    = errors.New("server not found")
	ErrServerConflict    = errors.New("server name or resource/port reservation conflicts")
	ErrServerPlacement   = errors.New("spark is missing, disabled, or does not provide the requested ports")
	ErrSparkReservations = errors.New("spark changes would invalidate existing server reservations")
)

// ServerRepository manages atomic placement and capacity reservations.
type ServerRepository struct{ db *gorm.DB }

// NewServerRepository creates a server repository.
func NewServerRepository(db *gorm.DB) *ServerRepository { return &ServerRepository{db: db} }

// AcknowledgeConfiguration records only a verified delivery fingerprint. The
// source values are checked under the database lock so a stale delivery cannot
// acknowledge a newer edit from another panel process.
func (r *ServerRepository) AcknowledgeConfiguration(ctx context.Context, expected model.Server, hash string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		current, err := NewServerRepository(tx).Get(ctx, expected.ID)
		if err != nil {
			return err
		}

		if current.EmberHash != expected.EmberHash || !model.SameRuntimeConfiguration(current.ServerConfiguration, expected.ServerConfiguration) {
			return ErrServerConflict
		}

		return tx.Model(&database.Server{}).Where("id = ?", expected.ID).Update("ember_hash", hash).Error
	})
}

// Save creates or updates a server under a serialized reservation transaction.
func (r *ServerRepository) Save(ctx context.Context, server model.Server, create bool) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		nameConflict := checkServerName(tx, server)
		if nameConflict != nil && !errors.Is(nameConflict, ErrServerConflict) {
			return nameConflict
		}

		if !create {
			var current database.Server
			if err := tx.Where("id = ?", server.ID).Take(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrServerNotFound
				}

				return err
			}

			configuration := model.ServerConfiguration{SparkID: current.SparkID, EmberID: current.EmberID, Image: current.Image, StartupCommand: current.StartupCommand, StartupOverride: current.StartupOverride, GameIntegration: current.GameIntegration, Variables: current.Variables, CPULimit: current.CPULimit, MemoryMB: current.MemoryMB, DiskMB: current.DiskMB, Ports: current.Ports}
			if model.SameRuntimeConfiguration(configuration, server.ServerConfiguration) && current.EmberHash == server.EmberHash {
				if nameConflict != nil {
					return nameConflict
				}

				return tx.Model(&database.Server{}).Where("id = ?", server.ID).Updates(map[string]any{"name": server.Name, "description": server.Description}).Error
			}

			if err := activeTransfer(tx, server.ID); err != nil {
				return err
			}
		}

		var spark database.Spark
		if err := tx.Where("id = ?", server.SparkID).Take(&spark).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrServerPlacement
			}

			return fmt.Errorf("read hosting Spark: %w", err)
		}

		if spark.Disabled && create {
			return ErrServerPlacement
		}

		existing, err := reservedServers(tx, server.SparkID, server.ID)
		if err != nil {
			return fmt.Errorf("read reservations: %w", err)
		}

		record := database.Server{ID: server.ID, Name: server.Name, Description: server.Description, SparkID: server.SparkID, EmberID: server.EmberID, EmberHash: server.EmberHash, Image: server.Image, StartupCommand: server.StartupCommand, StartupOverride: server.StartupOverride, GameIntegration: server.GameIntegration, Variables: server.Variables, CPULimit: server.CPULimit, MemoryMB: server.MemoryMB, DiskMB: server.DiskMB, Ports: server.Ports, CreatedAt: server.CreatedAt.Format(time.RFC3339Nano)}
		if create && server.OwnerUserID > 0 {
			record.OwnerUserID = &server.OwnerUserID
		}

		reservationConflict := checkReservations(spark, append(existing, record))
		if reservationConflict != nil && !errors.Is(reservationConflict, ErrServerConflict) {
			return reservationConflict
		}

		if err := errors.Join(nameConflict, reservationConflict); err != nil {
			return err
		}

		if create {
			err = tx.Omit("Spark", "Ember", "Owner").Create(&record).Error
		} else {
			result := tx.Model(&database.Server{}).Where("id = ?", server.ID).Select("Name", "Description", "EmberID", "EmberHash", "Image", "StartupCommand", "StartupOverride", "GameIntegration", "Variables", "CPULimit", "MemoryMB", "DiskMB", "Ports").Updates(&record)

			err = result.Error
			if err == nil && result.RowsAffected == 0 {
				return ErrServerNotFound
			}
		}

		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return serverConflictError(serverNameConflictMessage)
		}

		if err != nil {
			return fmt.Errorf("save server: %w", err)
		}

		return nil
	})
}

func checkReservations(spark database.Spark, servers []database.Server) error {
	var (
		conflicts []error
		cpu       float64
	)

	var memory, disk int64

	var ports []model.ServerPort

	for _, server := range servers {
		cpu += server.CPULimit
		memory += server.MemoryMB

		disk += server.DiskMB
		for _, port := range server.Ports {
			allowed := false

			for _, allocation := range spark.PortAllocations {
				if allocation.BindIP == port.BindIP && allocation.Protocol == port.Protocol && allocation.Start <= port.HostPort && port.HostPort <= allocation.End {
					allowed = true
					break
				}
			}

			if !allowed {
				return ErrServerPlacement
			}

			ip, _ := netip.ParseAddr(port.BindIP)
			for _, used := range ports {
				other, _ := netip.ParseAddr(used.BindIP)

				overlap := other == ip || (other.BitLen() == ip.BitLen() && (other.IsUnspecified() || ip.IsUnspecified()))
				if overlap && used.HostPort == port.HostPort && used.Protocol == port.Protocol {
					conflicts = append(conflicts, serverConflictError(fmt.Sprintf("Port %d/%s on %s is already reserved.", port.HostPort, port.Protocol, port.BindIP)))
					break
				}
			}

			ports = append(ports, port)
		}
	}

	if spark.CPULimit > 0 && cpu > spark.CPULimit+1e-9 {
		conflicts = append(conflicts, serverConflictError("The selected Spark has insufficient CPU capacity."))
	}

	if spark.MemoryMB > 0 && memory > spark.MemoryMB {
		conflicts = append(conflicts, serverConflictError("The selected Spark has insufficient memory capacity."))
	}

	if spark.DiskMB > 0 && disk > spark.DiskMB {
		conflicts = append(conflicts, serverConflictError("The selected Spark has insufficient disk capacity."))
	}

	return errors.Join(conflicts...)
}

// List returns saved servers with their display metadata.
func (r *ServerRepository) List(ctx context.Context) ([]model.Server, error) {
	return r.list(ctx, r.db, false)
}

func authorizedServers(query *gorm.DB, userID int64, admin bool) *gorm.DB {
	if admin {
		return query
	}

	return query.Where(`servers.owner_user_id = ? OR EXISTS (
SELECT 1 FROM server_access_grants grants
JOIN role_permissions permissions ON permissions.role_id = grants.access_role_id AND permissions.permission = ?
JOIN users ON users.id = ? AND users.deleted_at IS NULL AND users.disabled = false
WHERE grants.server_id = servers.id AND (grants.user_id = users.id OR grants.role_id = users.role_id))`, userID, accessperm.ServerView, userID)
}

// ListVisible filters inventory before loading definitions and redacts hidden values.
func (r *ServerRepository) ListVisible(ctx context.Context, userID int64, admin bool) ([]model.Server, error) {
	return r.list(ctx, authorizedServers(r.db, userID, admin), true)
}

func (r *ServerRepository) list(ctx context.Context, query *gorm.DB, visible bool) ([]model.Server, error) {
	var records []database.Server
	if err := query.WithContext(ctx).Preload("Spark").Preload("Ember").Order("created_at").Order("id").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}

	servers := make([]model.Server, 0, len(records))

	for _, record := range records {
		server, err := toServer(record)
		if err != nil {
			return nil, err
		}

		if record.OwnerUserID != nil {
			server.OwnerUserID = *record.OwnerUserID
		}

		if visible {
			server = model.VisibleServer(server, record.Ember.Definition)
		}

		servers = append(servers, server)
	}

	return servers, nil
}

// Get returns a server and its hosting definition metadata.
func (r *ServerRepository) Get(ctx context.Context, id string) (model.Server, error) {
	var record database.Server
	if err := r.db.WithContext(ctx).Preload("Spark").Preload("Ember").Where("id = ?", id).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Server{}, ErrServerNotFound
		}

		return model.Server{}, fmt.Errorf("read server: %w", err)
	}

	server, err := toServer(record)
	if err != nil {
		return server, err
	}

	if record.OwnerUserID != nil {
		server.OwnerUserID = *record.OwnerUserID
	}

	return server, nil
}

// Delete removes a saved configuration and releases its reservations.
func (r *ServerRepository) Delete(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		if err := activeTransfer(tx, id); err != nil {
			return err
		}

		result := tx.Where("id = ?", id).Delete(&database.Server{})
		if result.Error != nil {
			return fmt.Errorf("delete server: %w", result.Error)
		}

		if result.RowsAffected == 0 {
			return ErrServerNotFound
		}

		return nil
	})
}

func toServer(record database.Server) (model.Server, error) {
	created, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if err != nil {
		return model.Server{}, fmt.Errorf("read server timestamp: %w", err)
	}

	server := model.Server{ID: record.ID, EmberHash: record.EmberHash, ServerConfiguration: model.ServerConfiguration{Name: record.Name, Description: record.Description, SparkID: record.SparkID, EmberID: record.EmberID, Image: record.Image, StartupCommand: record.StartupCommand, StartupOverride: record.StartupOverride, GameIntegration: record.GameIntegration, Variables: record.Variables, CPULimit: record.CPULimit, MemoryMB: record.MemoryMB, DiskMB: record.DiskMB, Ports: record.Ports}, SparkName: record.Spark.Name, EmberName: record.Ember.Name, EmberArtURL: record.Ember.Definition.ArtURL, GameAddress: record.Spark.GameAddress, PortDescriptions: record.Ember.Definition.PortDescriptions(), Status: model.RuntimeStateNotInstalled, CreatedAt: created}
	server.SetEmberStatus(record.Ember.Definition)

	return server, nil
}
