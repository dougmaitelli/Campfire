package fileops

import "io/fs"

// ImportedMode selects private writable permissions for newly imported entries.
func ImportedMode(mode fs.FileMode) fs.FileMode {
	if mode.IsDir() || mode&0111 != 0 {
		return 0700
	}

	return 0600
}
