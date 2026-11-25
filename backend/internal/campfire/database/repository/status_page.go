package repository

import (
	"context"
	"crypto/rand"
	"errors"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/gameaddress"
	core "campfire/internal/campfire/integrations"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if err != nil {
		return nil, err
	}

	var bindings []database.ServerIntegration
	if err := r.db.WithContext(ctx).Select("status_page_id, destination_id, enabled").Where("status_page_id IN (SELECT id FROM status_pages)").Find(&bindings).Error; err != nil {
		return nil, err
	}

	channels := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		channel := ""
		if binding.Enabled {
			channel = binding.DestinationID
		}

		channels[binding.StatusPageID] = channel
	}

	pages := make([]statuspage.Page, 0, len(rows))
	for _, row := range rows {
		page := statusPageView(row)
		if channel, ok := channels[row.ID]; ok {
			page.DiscordChannelID = &channel
		}

		pages = append(pages, page)
	}

	return pages, err
}

// Save checks publication membership and replaces it atomically. Entry IDs remain
// stable while membership remains unchanged and never reveal server identifiers.
func (r *StatusPageRepository) Save(ctx context.Context, id string, input statuspage.Input) (statuspage.Page, error) {
	return r.SaveWithDiscord(ctx, id, input, core.Connection{})
}

// SaveWithDiscord commits the page and its optional publishing change together.
func (r *StatusPageRepository) SaveWithDiscord(ctx context.Context, id string, input statuspage.Input, checked core.Connection) (statuspage.Page, error) {
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
		channel, err := savePageDestination(ctx, tx, row.ID, input.DiscordChannelID, checked)
		result.DiscordChannelID = channel

		return err
	})

	return result, err
}

// Delete removes a publication and its memberships without deleting servers.
func (r *StatusPageRepository) Delete(ctx context.Context, id string) error {
	return database.Transaction(r.db.WithContext(ctx), func(tx *gorm.DB) error {
		off := ""
		if _, err := savePageDestination(ctx, tx, id, &off, core.Connection{}); err != nil {
			return err
		}

		var page database.StatusPage

		result := tx.Clauses(clause.Returning{Columns: []clause.Column{{Name: "title"}}}).Where("id = ?", id).Delete(&page)
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return statuspage.ErrNotFound
		}

		return tx.Model(&database.ServerIntegration{}).Where("status_page_id = ?", id).Update("deleted_page_title", page.Title).Error
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
	return r.published(ctx, "pages.slug = ?", slug, true)
}

// PublishedByID follows a card's stable publication even after a slug change.
func (r *StatusPageRepository) PublishedByID(ctx context.Context, id string) (string, []PublishedTarget, error) {
	return r.published(ctx, "pages.id = ?", id, false)
}

func (r *StatusPageRepository) published(ctx context.Context, predicate, value string, gameDetails bool) (string, []PublishedTarget, error) {
	// Filter live memberships before the outer join so an empty page (including
	// one whose servers are all being deleted) still has a title. One statement
	// keeps the title and selection consistent without acquiring a write lock.
	members := r.db.WithContext(ctx).Table("status_page_entries AS membership").
		Joins("JOIN servers ON servers.id = membership.server_id").
		Joins("JOIN sparks ON sparks.id = servers.spark_id").
		Where("servers.deletion_pending = ?", false)
	columns := "membership.id, membership.page_id, membership.name, membership.position, membership.server_id, servers.spark_id, sparks.disabled"

	if gameDetails {
		ports := "COALESCE(json_extract(embers.definition, '$.required_ports'), '[]')"
		schema := "COALESCE(json_extract(embers.definition, '$.schema'), 2)"

		art := "COALESCE(json_extract(embers.definition, '$.artUrl'), '')"
		if r.db.Name() == "postgres" {
			art = "COALESCE(embers.definition::jsonb ->> 'artUrl', '')"
			ports = "COALESCE(embers.definition::jsonb ->> 'required_ports', '[]')"
			schema = "COALESCE((embers.definition::jsonb ->> 'schema')::integer, 2)"
		}

		members = members.Joins("JOIN embers ON embers.id = servers.ember_id")
		columns += ", servers.game_integration, servers.game_address, sparks.game_address AS default_host, servers.ports, " + ports + " AS required_ports, " + schema + " AS ember_schema, " + art + " AS art_url"
	}

	var rows []struct {
		Title           string
		Present         bool
		PublishedTarget `gorm:"embedded"`
		DefaultHost     string
		EmberSchema     int
		Ports           []model.ServerPort   `gorm:"serializer:json"`
		RequiredPorts   []model.RequiredPort `gorm:"serializer:json"`
	}

	err := r.db.WithContext(ctx).Table("status_pages AS pages").
		Select("pages.title, entries.id IS NOT NULL AS present, entries.*").
		Joins("LEFT JOIN (?) AS entries ON entries.page_id = pages.id", members.Select(columns)).
		Where(predicate, value).
		Order("entries.position, entries.id").Limit(statuspage.MaxEntries).Scan(&rows).Error
	if err != nil {
		return "", nil, err
	}

	if len(rows) == 0 {
		return "", nil, statuspage.ErrNotFound
	}

	entries := make([]PublishedTarget, 0, len(rows))
	for _, row := range rows {
		if !row.Present {
			continue
		}

		if gameDetails {
			definition := (model.Ember{Schema: row.EmberSchema, RequiredPorts: row.RequiredPorts}).Normalized()
			row.GameAddress = gameaddress.Resolve(row.GameAddress, row.DefaultHost, model.PrimaryHostPort(row.Ports, definition.RequiredPorts))
		}

		entries = append(entries, row.PublishedTarget)
	}

	return rows[0].Title, entries, nil
}
