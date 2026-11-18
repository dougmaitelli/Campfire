package fileops

import (
	"errors"
	"io"
	"os"

	"campfire/internal/shared/model"
)

// The original source identity stays protected even if it is renamed or linked
// elsewhere while the operation is waiting. sourceInfo is immutable after scan
// initialization; conflict processing is serialized by the session's locks.
func (s *extractionSession) protectedDestination(root *os.Root, target string) (bool, error) {
	if target == s.source {
		return true, nil
	}

	info, err := root.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, Failure(model.FileIO, "Could not inspect extraction destination.")
	}

	return s.sourceInfo != nil && os.SameFile(s.sourceInfo, info), nil
}

// Only undecided members are buffered, outside the game-data volume. A nil
// reader means this entry already has buffered contents (or is a directory).
func (s *extractionSession) buffer(item *extractionItem, reader io.Reader) error {
	if item.entry.Kind != model.FileKindFile || reader == nil {
		return nil
	}

	s.mu.Lock()

	var err error
	if s.temporary == "" {
		s.temporary, err = os.MkdirTemp("", "campfire-extraction-")
	}

	temporary := s.temporary
	s.mu.Unlock()

	if err != nil {
		return err
	}

	file, err := os.CreateTemp(temporary, "entry-")
	if err != nil {
		return err
	}

	_, copyErr := io.CopyN(file, contextReader{s.ctx, reader}, item.entry.Size)
	closeErr := file.Close()

	if copyErr != nil {
		return copyErr
	}

	if closeErr != nil {
		return closeErr
	}

	s.mu.Lock()
	item.temporary = file.Name()
	s.mu.Unlock()

	return nil
}

// A destination can change during an allocation scan, including for a member
// which was initially fresh. Keep its incoming contents for an explicit answer.
// The caller holds fsMu, not the state lock.
func (s *extractionSession) changedDestination(root *os.Root, index int, reader io.Reader, existing *model.FileEntry, revision string) error {
	item := s.items[index]

	protected, err := s.protectedDestination(root, item.target)
	if err != nil {
		return err
	}

	if err := s.buffer(item, reader); err != nil {
		return err
	}

	s.mu.Lock()
	s.setConflict(index, existing, revision, true, protected)
	s.message = "The destination changed. Review this conflict again."
	s.mu.Unlock()

	return nil
}
