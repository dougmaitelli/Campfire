//go:build !production

package webui

import "io/fs"

func assets() fs.FS { return nil }
