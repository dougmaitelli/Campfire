package runtime

import (
	"context"
	"errors"
	"math"
	"os"

	"campfire/internal/shared/model"
)

var errProjectAccountingUnavailable = errors.New("game data has no managed quota project")

// Native providers are optional; quota implementations without a counter retain
// the bounded scan. Keep the built-in provider compile-checked against the hook.
type diskUsageProvider interface {
	DiskUsage(string) (int64, error)
}

var _ diskUsageProvider = (*projectQuotas)(nil)

// DiskUsage reads the kernel's byte charge, including allocated metadata and
// deleted-open files. It never assigns projects or changes limits while sampling.
func (q *projectQuotas) DiskUsage(directory string) (int64, error) {
	file, attrs, owner, err := q.open(directory)
	if errors.Is(err, os.ErrNotExist) {
		return 0, errProjectAccountingUnavailable
	}

	if err != nil {
		return 0, err
	}

	defer func() { _ = file.Close() }()

	if attrs.Project == 0 {
		return 0, errProjectAccountingUnavailable
	}

	if attrs.Flags&0x200 == 0 || !q.owned(attrs.Project, owner) {
		return 0, errors.New("game data is not owned by its quota registry")
	}

	limit, err := q.kernel.Get(file, attrs.Project)
	if err != nil {
		return 0, err
	}
	// Q_GETQUOTA dqb_curspace is bytes; dqb_bhardlimit uses 1 KiB units.
	if limit.Space > math.MaxInt64 {
		return 0, errors.New("invalid project space counter")
	}

	return int64(limit.Space), nil
}

func (m *Manager) measureDisk(ctx context.Context, id string) (int64, model.DiskAccounting, error) {
	if err := ctx.Err(); err != nil {
		return 0, "", err
	}

	if provider, ok := m.Quota.(diskUsageProvider); ok {
		volume, err := m.volume(id)
		if err != nil {
			return 0, "", err
		}

		bytes, err := provider.DiskUsage(volume)
		if err == nil {
			return bytes, model.DiskAccountingProject, ctx.Err()
		}
		// Unprepared/missing game data has no project to measure. Once a project
		// exists, errors must retain the old reading as stale, not switch units.
		if !errors.Is(err, errProjectAccountingUnavailable) {
			return 0, model.DiskAccountingProject, err
		}
	}

	bytes, err := measureData(ctx, m.root(id))

	return bytes, model.DiskAccountingLogical, err
}
