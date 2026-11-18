package fileops

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"campfire/internal/shared/model"
)

// archiveMembers shares member safety and expansion checks, not orchestration.
// Interactive extraction separately counts implicit parents in its result limit.
type archiveMembers struct {
	seen         map[string]bool
	count, limit int
	total, bytes int64
}

func newArchiveMembers(limit int, bytes int64) *archiveMembers {
	return &archiveMembers{seen: make(map[string]bool), limit: limit, bytes: bytes}
}

// accept returns an empty name for a harmless archive-root directory header.
func (a *archiveMembers) accept(name string, size int64, mode fs.FileMode) (string, error) {
	a.count++
	if a.count > a.limit || size > a.bytes-a.total {
		return "", Failure(model.FileLimitExceeded, "Archive exceeds its expansion size or entry limit. Completed entries were kept.")
	}

	name = strings.TrimSuffix(name, "/")
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}

	if (name == "" || name == ".") && mode.IsDir() && size == 0 {
		return "", nil
	}

	if !ValidPath(name, false) || (!mode.IsDir() && !mode.IsRegular()) || size < 0 || (mode.IsDir() && size != 0) {
		return "", Failure(model.FileInvalid, "Archive contains unsafe or unsupported entries.")
	}

	if a.seen[name] {
		return "", Failure(model.FileInvalid, fmt.Sprintf("Archive contains duplicate path %q. Completed entries were kept; rename or remove duplicate entries and try again.", name))
	}

	a.seen[name] = true
	a.total += size

	return name, nil
}

// archiveAllocation accounts for this operation's writes without walking game
// data for every member. Periodic reconciliation includes concurrent edits;
// runtime project quotas, where supported, remain the hard filesystem limit.
// Callers serialize reservations with their filesystem writes.
type archiveAllocation struct {
	limit, used int64
	refreshed   time.Time
	usage       func(context.Context, string) (int64, error)
}

const archiveAllocationRefresh = time.Second

func newArchiveAllocation(ctx context.Context, volume string, limitMB int64) (*archiveAllocation, error) {
	a := &archiveAllocation{limit: limitMB * 1048576, usage: FileUsage}
	if err := a.refresh(ctx, volume); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *archiveAllocation) refresh(ctx context.Context, volume string) error {
	if a.limit == 0 || (!a.refreshed.IsZero() && time.Since(a.refreshed) < archiveAllocationRefresh) {
		return nil
	}

	used, err := a.usage(ctx, volume)
	if err != nil {
		return err
	}

	a.used, a.refreshed = used, time.Now()

	return nil
}

func (a *archiveAllocation) reserve(ctx context.Context, volume string, oldSize, newSize int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := a.refresh(ctx, volume); err != nil {
		return err
	}

	return a.reserveBytes(oldSize, newSize)
}

// reserveBytes does no filesystem IO. Conflicting writes refresh allocation
// before their final destination check, then reserve without another tree walk.
func (a *archiveAllocation) reserveBytes(oldSize, newSize int64) error {
	if a.limit == 0 {
		return nil
	}

	used := a.used - oldSize + newSize
	if used > a.limit {
		return Failure(model.FileLimitExceeded, "Extracted files would exceed the server disk allocation.")
	}

	a.used = used

	return nil
}
