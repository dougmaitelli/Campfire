package archive

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWorkflowAndSharedPackageBoundaries(t *testing.T) {
	for _, scope := range []struct {
		directory string
		workflow  bool
	}{
		{"..", true},
		{filepath.Join("..", "..", "..", "..", "shared", "archive"), false},
	} {
		files, err := filepath.Glob(filepath.Join(scope.directory, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatal("missing boundary sources", scope.directory, err)
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

				codecLibrary := strings.HasPrefix(path, "archive/") || strings.HasPrefix(path, "compress/") || strings.HasPrefix(path, "github.com/nwaples/rardecode") || strings.HasPrefix(path, "github.com/ulikunitz/xz") || strings.HasPrefix(path, "github.com/xi2/xz") || strings.HasPrefix(path, "github.com/pierrec/lz4") || strings.HasPrefix(path, "github.com/klauspost/compress")
				if scope.workflow && codecLibrary {
					t.Errorf("filesystem workflow %s imports format-specific codec %s", name, path)
				}

				if !scope.workflow && (strings.HasPrefix(path, "campfire/internal/spark/") || strings.HasPrefix(path, "github.com/nwaples/rardecode") || strings.HasPrefix(path, "github.com/xi2/xz")) {
					t.Errorf("shared writer %s depends on Spark-only extraction %s", name, path)
				}
			}
		}
	}
}
