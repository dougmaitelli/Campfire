package repository

import (
	"context"
	"crypto/rand"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/gameaddress"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// StatusPageRepository persists explicit public memberships.
type StatusPageRepository struct{ db *gorm.DB }

// NewStatusPageRepository uses the panel's shared database pool.
func NewStatusPageRepository(db *gorm.DB) *StatusPageRepository { return &StatusPageRepository{db: db} }

func statusPageView(row database.StatusPage) statuspage.Page {
	entries := make([]statuspage.Selection, 0, len(row.Entries))
	for _, entry := range row.Entries {
		entries = append(entries, statuspage.Selection{ServerID: entry.ServerID, Name: entry.Name})
	}

	return statuspage.Page{ID: row.ID, Input: statuspage.Input{Title: row.Title, Slug: row.Slug, Entries: entries}}
}

// List returns the bounded management collection with ordered entries.
func (r *StatusPageRepository) List(ctx context.Context) ([]statuspage.Page, error) {
	var rows []database.StatusPage

	err := r.db.WithContext(ctx).Order("title, id").Limit(statuspage.MaxPages).Preload("Entries", func(tx *gorm.DB) *gorm.DB { return tx.Order("position, id") }).Find(&rows).Error

	pages := make([]statuspage.Page, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, statusPageView(row))
	}

	return pages, err
}

// Save checks publication membership and replaces it atomically. Entry IDs remain
// stable while membership remains unchanged and never reveal server identifiers.
func (r *StatusPageRepository) Save(ctx context.Context, id string, input statuspage.Input) (statuspage.Page, error) {
	var result statuspage.Page

	err := database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		var (
			row   database.StatusPage
			count int64
		)
		if id == "" {
			if err := tx.Model(&database.StatusPage{}).Count(&count).Error; err != nil {
				return err
			}

			if count >= statuspage.MaxPages {
				return statuspage.ErrLimit
			}

			row.ID = rand.Text()
		} else {
			if err := tx.Preload("Entries").Where("id = ?", id).Take(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return statuspage.ErrNotFound
				}

				return err
			}
		}

		if err := tx.Model(&database.StatusPage{}).Where("slug = ? AND id <> ?", input.Slug, row.ID).Count(&count).Error; err != nil {
			return err
		}

		if count > 0 {
			return statuspage.ErrConflict
		}

		ids := make([]string, 0, len(input.Entries))
		for _, e := range input.Entries {
			ids = append(ids, e.ServerID)
		}

		if len(ids) > 0 {
			if err := tx.Model(&database.Server{}).Where("id IN ? AND deletion_pending = ?", ids, false).Count(&count).Error; err != nil {
				return err
			}

			if count != int64(len(ids)) {
				return statuspage.ErrInput
			}
		}

		existing := make(map[string]string, len(row.Entries))
		for _, e := range row.Entries {
			existing[e.ServerID] = e.ID
		}

		row.Title, row.Slug = input.Title, input.Slug
		if id == "" {
			if err := tx.Omit("Entries").Create(&row).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&database.StatusPage{}).Where("id = ?", id).Updates(map[string]any{"title": row.Title, "slug": row.Slug}).Error; err != nil {
				return err
			}

			if err := tx.Where("page_id = ?", id).Delete(&database.StatusPageEntry{}).Error; err != nil {
				return err
			}
		}

		row.Entries = nil
		for position, e := range input.Entries {
			entryID := existing[e.ServerID]
			if entryID == "" {
				entryID = rand.Text()
			}

			row.Entries = append(row.Entries, database.StatusPageEntry{ID: entryID, PageID: row.ID, ServerID: e.ServerID, Name: e.Name, Position: position})
		}

		if len(row.Entries) > 0 {
			if err := tx.Omit("Server").Create(&row.Entries).Error; err != nil {
				return err
			}
		}

		result = statusPageView(row)

		return nil
	})

	return result, err
}

// Delete removes a publication and its memberships without deleting servers.
func (r *StatusPageRepository) Delete(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		result := tx.Where("id = ?", id).Delete(&database.StatusPage{})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return statuspage.ErrNotFound
		}

		return nil
	})
}

// PublishedTarget is an internal projection. It must never be serialized publicly.
type PublishedTarget struct {
	ID, Name, ServerID, SparkID string
	Disabled                    bool
	ArtURL                      string
	GameAddress                 string
	GameIntegration             bool
}

// Published resolves current membership and placement on every anonymous read.
func (r *StatusPageRepository) Published(ctx context.Context, slug string) (string, []PublishedTarget, error) {
	// Filter live memberships before the outer join so an empty page (including
	// one whose servers are all being deleted) still has a title. One statement
	// keeps the title and selection consistent without acquiring a write lock.
	ports := "COALESCE(json_extract(embers.definition, '$.required_ports'), '[]')"

	art := "COALESCE(json_extract(embers.definition, '$.artUrl'), '')"
	if r.db.Name() == "postgres" {
		art = "COALESCE(embers.definition::jsonb ->> 'artUrl', '')"
		ports = "COALESCE(embers.definition::jsonb ->> 'required_ports', '[]')"
	}

	members := r.db.WithContext(ctx).Table("status_page_entries AS membership").
		Select("membership.id, membership.page_id, membership.name, membership.position, membership.server_id, servers.spark_id, servers.game_integration, sparks.disabled, servers.game_address, sparks.game_address AS default_host, servers.ports, "+ports+" AS required_ports, "+art+" AS art_url").
		Joins("JOIN servers ON servers.id = membership.server_id").
		Joins("JOIN sparks ON sparks.id = servers.spark_id").
		Joins("JOIN embers ON embers.id = servers.ember_id").
		Where("servers.deletion_pending = ?", false)

	var rows []struct {
		Title           string
		Present         bool
		PublishedTarget `gorm:"embedded"`
		DefaultHost     string
		Ports           []model.ServerPort   `gorm:"serializer:json"`
		RequiredPorts   []model.RequiredPort `gorm:"serializer:json"`
	}

	err := r.db.WithContext(ctx).Table("status_pages AS pages").
		Select("pages.title, entries.id IS NOT NULL AS present, entries.id, entries.name, entries.server_id, entries.spark_id, entries.disabled, entries.game_integration, entries.game_address, entries.default_host, entries.ports, entries.required_ports, entries.art_url").
		Joins("LEFT JOIN (?) AS entries ON entries.page_id = pages.id", members).
		Where("pages.slug = ?", slug).
		Order("entries.position, entries.id").Limit(statuspage.MaxEntries).Scan(&rows).Error
	if err != nil {
		return "", nil, err
	}

	if len(rows) == 0 {
		return "", nil, statuspage.ErrNotFound
	}

	entries := make([]PublishedTarget, 0, len(rows))
	for _, row := range rows {
		if row.Present {
			row.GameAddress = gameaddress.Resolve(row.GameAddress, row.DefaultHost, model.PrimaryHostPort(row.Ports, row.RequiredPorts))
			entries = append(entries, row.PublishedTarget)
		}
	}

	return rows[0].Title, entries, nil
}
