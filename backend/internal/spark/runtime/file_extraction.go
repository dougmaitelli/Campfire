package runtime

import (
	"os"

	"campfire/internal/spark/runtime/fileops"
)

func (m *Manager) newFileOperations() *fileops.Manager {
	return fileops.New(fileops.Host{
		FileOwner:          func(file *os.File, user string) error { return m.FileOwner(file, user) },
		LogicalDiskLimitMB: m.logicalDiskLimitMB,
	})
}
