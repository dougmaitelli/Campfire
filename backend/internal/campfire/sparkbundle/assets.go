//go:build production

package sparkbundle

import (
	"embed"
	"io/fs"
)

//go:embed dist
var bundled embed.FS

func assets() fs.FS {
	files, err := fs.Sub(bundled, "dist")
	if err != nil {
		panic(err)
	}

	return files
}
