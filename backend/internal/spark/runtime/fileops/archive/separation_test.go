package archive

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFilesystemWorkflowsDoNotImportCodecs(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal("missing workflow sources", err)
	}

	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}

		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}

			for _, codec := range []string{"archive/", "compress/", "github.com/nwaples/rardecode", "github.com/ulikunitz/xz", "github.com/xi2/xz", "github.com/pierrec/lz4", "github.com/klauspost/compress"} {
				if strings.HasPrefix(path, codec) {
					t.Errorf("filesystem workflow %s imports format-specific codec %s", name, path)
				}
			}
		}
	}
}
