//go:build production

package webui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var frontend embed.FS

func assets() fs.FS {
	files, err := fs.Sub(frontend, "dist")
	if err != nil {
		panic(err)
	}

	return files
}
