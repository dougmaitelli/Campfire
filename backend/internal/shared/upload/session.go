// Package upload owns temporary upload buffers. Callers own authorization,
// admission limits and final publication policy; incomplete buffers never become
// public files merely because their transfer reached 100 percent.
package upload

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrTooManyRanges bounds metadata used by random-access uploads.
var ErrTooManyRanges = errors.New("upload has too many disjoint ranges")

// Session is accessed under its owner's lock. Sequential sessions reject gaps;
// random-access sessions track every written range for SFTP-style uploads.
type Session struct {
	File                *os.File
	Size, Limit, Offset int64
	sequential          bool
	ranges              [][2]int64
	cleanup             func() error
}

// Create makes private, named temporary storage. Close or Prune removes
// incomplete buffers; finalized archives have their own durable names.
func Create(directory, prefix string, size, limit int64) (*Session, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}

	file, err := os.CreateTemp(directory, prefix)
	if err != nil {
		return nil, err
	}

	cleanup := func() error { return os.Remove(file.Name()) }

	return &Session{File: file, Size: size, Limit: limit, sequential: true, cleanup: cleanup}, nil
}

// CreateAt anchors temporary storage to a filesystem root for safe publication
// on the destination filesystem. No caller-supplied path escapes this root.
func CreateAt(root *os.Root, name string, size, limit int64) (*Session, error) {
	pinned, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}

	file, err := pinned.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		_ = pinned.Close()
		return nil, err
	}

	return &Session{File: file, Size: size, Limit: limit, cleanup: func() error {
		err := pinned.Remove(name)
		_ = pinned.Close()

		return err
	}}, nil
}

// Write accepts a bounded chunk and records its coverage only after a full write.
func (s *Session) Write(offset int64, data []byte) error {
	if s.File == nil || offset < 0 || offset > s.Limit-int64(len(data)) || s.Size >= 0 && offset+int64(len(data)) > s.Size || s.sequential && offset != s.Offset {
		return errors.New("invalid upload offset or size")
	}

	n, err := s.File.WriteAt(data, offset)
	if err != nil {
		return err
	}

	if n != len(data) {
		return io.ErrShortWrite
	}

	s.Offset = max(s.Offset, offset+int64(n))

	if len(data) == 0 {
		return nil
	}

	interval := [2]int64{offset, offset + int64(n)}

	merged := make([][2]int64, 0, len(s.ranges)+1)
	for _, current := range s.ranges {
		if current[1] < interval[0] {
			merged = append(merged, current)
		} else if interval[1] < current[0] {
			merged = append(merged, interval)
			interval = current
		} else {
			interval[0] = min(interval[0], current[0])
			interval[1] = max(interval[1], current[1])
		}
	}

	merged = append(merged, interval)
	if len(merged) > 100000 {
		return ErrTooManyRanges
	}

	s.ranges = merged

	return nil
}

// Complete checks both file size and written coverage, not just the final offset.
func (s *Session) Complete() error {
	if s.File == nil {
		return errors.New("upload is unavailable")
	}

	info, err := s.File.Stat()
	if err != nil {
		return err
	}

	size := info.Size()
	if size > s.Limit || s.Size >= 0 && (size != s.Size || size > 0 && (len(s.ranges) != 1 || s.ranges[0] != [2]int64{0, size})) {
		return errors.New("upload is incomplete")
	}

	return nil
}

// PublishNoReplace makes a complete buffer durable without overwriting another
// archive. Repeating publication of this same buffer is idempotent. Destination
// validation and catalog registration remain the caller's responsibility.
func (s *Session) PublishNoReplace(destination string) error {
	if err := s.Complete(); err != nil {
		return err
	}

	if err := s.File.Sync(); err != nil {
		return err
	}

	if err := os.Link(s.File.Name(), destination); err != nil {
		source, sourceErr := s.File.Stat()

		target, targetErr := os.Lstat(destination)
		if !errors.Is(err, os.ErrExist) || sourceErr != nil || targetErr != nil || !target.Mode().IsRegular() || !os.SameFile(source, target) {
			return err
		}
	}

	directory, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return err
	}

	defer func() { _ = directory.Close() }()

	return directory.Sync()
}

// Prune removes only expired temporary files, never finalized archives. Active
// session names are excluded even while their owner validates or publishes them.
func Prune(directory, prefix string, before time.Time, active map[string]bool) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}

	for _, entry := range entries {
		name := filepath.Join(directory, entry.Name())
		if active[name] || !strings.HasPrefix(entry.Name(), prefix) || !entry.Type().IsRegular() {
			continue
		}

		info, err := entry.Info()
		if err == nil && info.ModTime().Before(before) {
			_ = os.Remove(name)
		}
	}
}

// Close is idempotent after cancellation, expiry or destination publication.
func (s *Session) Close() {
	if s == nil {
		return
	}

	if s.File != nil {
		_ = s.File.Close()
		s.File = nil
	}

	if s.cleanup != nil {
		_ = s.cleanup()
		s.cleanup = nil
	}
}
