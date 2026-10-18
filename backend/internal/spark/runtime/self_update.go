package runtime

import (
	"errors"
	"os"
)

// CanUpdate requires all asynchronous work, uploads and transfer fences to be idle.
// Agent RPCs are serialized, so no new operation can begin during the update RPC.
func (m *Manager) CanUpdate() error {
	m.mu.Lock()
	if m.closed || len(m.transferTasks) > 0 {
		m.mu.Unlock()
		return errors.New("wait for active Spark operations before updating")
	}

	entries := make([]*entry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}
	m.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
		busy := entry.Status.Busy || len(entry.uploads) > 0
		entry.mu.Unlock()

		if busy {
			return errors.New("wait for server operations and file uploads before updating Spark")
		}
	}

	items, err := os.ReadDir(m.Directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	for _, item := range items {
		if !item.IsDir() || !safeID.MatchString(item.Name()) {
			continue
		}

		transfer, err := m.localTransfer(item.Name())
		if err != nil {
			return err
		}

		if transfer != nil && transfer.State != "active" && transfer.State != "released" && transfer.State != "retired" && transfer.State != "discarded" {
			return errors.New("finish the server transfer before updating Spark")
		}
	}

	return nil
}
