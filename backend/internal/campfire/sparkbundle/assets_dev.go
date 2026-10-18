//go:build !production

package sparkbundle

import "io/fs"

func assets() fs.FS { return nil }
