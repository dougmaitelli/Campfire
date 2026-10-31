package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

var dockerImageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// rememberImage records provenance rather than treating every host image as ours.
// Keep earlier IDs when tags change so unused versions can be reclaimed later.
func (m *Manager) rememberImage(serverID, image string, e *entry) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := m.Docker.Run(ctx, nil, "image", "inspect", "--format", "{{.Id}}", image)

	id := strings.TrimSpace(output)
	if err != nil || !dockerImageID.MatchString(id) {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if !slices.Contains(e.Images, id) {
		e.Images = append(e.Images, id)
		if err := m.persist(serverID, e); err != nil {
			slog.Warn("Could not save Campfire image inventory", "server", serverID, "error", err)
		}
	}
}

// Prune removes only resources attributable to Campfire. Bind-mounted game data
// and backups are never traversed or removed. Current stopped containers remain.
func (m *Manager) Prune(ctx context.Context) (model.PruneResult, error) {
	result := model.PruneResult{}
	if !m.maintenance.TryLock() {
		return result, errors.New("wait for active server operations to finish before pruning")
	}
	defer m.maintenance.Unlock()

	images := make(map[string]bool)
	deleted := make(map[string]bool)

	dirs, err := os.ReadDir(m.Directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, errors.New("could not read Campfire resource inventory")
	}

	for _, dir := range dirs {
		if !dir.IsDir() || !utils.IsValidResourceID(dir.Name()) {
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.root(dir.Name()), "state.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		var saved entry
		if err != nil || json.Unmarshal(data, &saved) != nil {
			return result, errors.New("could not read Campfire resource inventory")
		}

		deleted[dir.Name()] = saved.Status.State == "deleted"
		for _, id := range saved.Images {
			if dockerImageID.MatchString(id) {
				images[id] = true
			}
		}
	}

	output, err := m.Docker.Run(ctx, nil, "container", "ls", "--all", "--filter", "label=campfire.server", "--format", `{{json .}}`)
	if err != nil {
		return result, errors.New("could not list Campfire containers")
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}

		var row struct{ ID, Names, State string }
		if json.Unmarshal([]byte(line), &row) != nil || !utils.IsValidResourceID(row.ID) {
			return result, errors.New("invalid Docker container inventory")
		}

		// Inspect only selected fields; never bring environment secrets into output.
		image, err := m.Docker.Run(ctx, nil, "inspect", "--format", "{{.Image}}", row.ID)
		if err != nil {
			return result, errors.New("could not inspect a Campfire container")
		}

		if id := strings.TrimSpace(image); dockerImageID.MatchString(id) {
			images[id] = true
		}

		serverID := strings.TrimPrefix(row.Names, "campfire-")

		removable := strings.HasSuffix(serverID, "-install") || deleted[serverID]
		if !removable || (row.State != "exited" && row.State != "dead" && row.State != "created") {
			continue
		}

		if _, err := m.Docker.Run(ctx, nil, "container", "rm", row.ID); err != nil {
			return result, errors.New("could not remove an unused Campfire container; cleanup may be partially complete")
		}

		result.Containers++
	}

	for id := range images {
		used, err := m.Docker.Run(ctx, nil, "container", "ls", "--all", "--quiet", "--filter", "ancestor="+id)
		if err != nil {
			return result, errors.New("could not check Docker image references")
		}

		if strings.TrimSpace(used) != "" {
			continue
		}

		// No force: Docker also protects racing users and images with shared tags.
		if _, err := m.Docker.Run(ctx, nil, "image", "rm", "--no-prune", id); err == nil {
			result.Images++
		}
	}

	output, err = m.Docker.Run(ctx, nil, "volume", "ls", "--quiet", "--filter", "label=campfire.server")
	if err != nil {
		return result, errors.New("could not list Campfire volumes")
	}

	for _, volume := range strings.Fields(output) {
		if !utils.IsValidResourceID(volume) {
			return result, errors.New("invalid Docker volume inventory")
		}

		used, err := m.Docker.Run(ctx, nil, "container", "ls", "--all", "--quiet", "--filter", "volume="+volume)
		if err != nil {
			return result, errors.New("could not check Docker volume references")
		}

		if strings.TrimSpace(used) == "" {
			if _, err := m.Docker.Run(ctx, nil, "volume", "rm", volume); err != nil {
				return result, errors.New("could not remove an unused Campfire volume; cleanup may be partially complete")
			}

			result.Volumes++
		}
	}

	return result, ctx.Err()
}
