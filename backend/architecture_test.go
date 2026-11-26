package architecture_test

import (
	"errors"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Check all production-owned source files, including platform-specific and
// untagged development builds. Tests may consume development fixtures; product
// code must not depend on the simulator or the other product's implementation.
func TestProductionSourceOwnership(t *testing.T) {
	for _, owner := range []string{"campfire", "spark", "shared"} {
		t.Run(owner, func(t *testing.T) {
			err := filepath.WalkDir(filepath.Join("internal", owner), func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
					return nil
				}

				file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
				if err != nil {
					return err
				}

				for _, spec := range file.Imports {
					path, err := strconv.Unquote(spec.Path.Value)
					if err != nil {
						return err
					}

					if strings.HasPrefix(path, "campfire/") && !withinPackage(path, "campfire/internal/"+owner) && !withinPackage(path, "campfire/internal/shared") {
						t.Errorf("%s imports %s outside its production ownership boundary", name, path)
					}
				}

				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExecutableDependencyBoundaries(t *testing.T) {
	for _, tags := range [][]string{nil, {"production"}} {
		for _, owner := range []string{"campfire", "spark"} {
			t.Run(owner+"/"+strings.Join(tags, ","), func(t *testing.T) {
				for path := range internalDependencies(t, "campfire/cmd/"+owner, tags) {
					if strings.HasPrefix(path, "campfire/internal/") && !withinPackage(path, "campfire/internal/"+owner) && !withinPackage(path, "campfire/internal/shared") {
						t.Errorf("%s executable includes %s", owner, path)
					}
				}
			})
		}
	}

	harness := internalDependencies(t, "campfire/e2e", []string{"e2e"})
	if !harness["campfire/internal/devfixture"] || harness["campfire/internal/devmock"] {
		t.Fatal("browser harness must use fixtures without the interactive mock runtime")
	}
}

func TestSharedPackagesHaveBothProductionConsumers(t *testing.T) {
	panel := internalDependencies(t, "campfire/cmd/campfire", []string{"production"})
	spark := internalDependencies(t, "campfire/cmd/spark", []string{"production"})

	err := filepath.WalkDir("internal/shared", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		path := "campfire/" + filepath.ToSlash(filepath.Dir(name))
		if !panel[path] || !spark[path] {
			t.Errorf("%s lacks both production consumers; mock/test reuse alone does not justify shared ownership", path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevelopmentPackagesAreExcludedFromProduction(t *testing.T) {
	ctx := build.Default
	ctx.BuildTags = []string{"production", "e2e"}

	for _, root := range []string{"internal/devmock", "internal/devfixture", "cmd/campfire-mock", "e2e"} {
		err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if entry.IsDir() || !strings.HasSuffix(name, ".go") {
				return nil
			}

			included, err := ctx.MatchFile(filepath.Dir(name), filepath.Base(name))
			if included {
				t.Errorf("development source %s is enabled in production", name)
			}

			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = ctx.ImportDir(filepath.FromSlash(root), 0)

		var excluded *build.NoGoError
		if !errors.As(err, &excluded) {
			t.Errorf("development package %s not excluded: %v", root, err)
		}
	}
}

// Resolve local source imports using Go's build constraints, without requiring
// generated release embeds or linking external libraries. Source ownership above
// also checks files excluded on the current platform.
func internalDependencies(t *testing.T, root string, tags []string) map[string]bool {
	t.Helper()

	ctx := build.Default
	ctx.BuildTags = tags
	seen := make(map[string]bool)

	var visit func(string)

	visit = func(path string) {
		if seen[path] || !strings.HasPrefix(path, "campfire/") {
			return
		}

		seen[path] = true

		pkg, err := ctx.ImportDir(filepath.FromSlash(strings.TrimPrefix(path, "campfire/")), 0)
		if err != nil {
			t.Fatalf("load %s (%v): %v", path, tags, err)
		}

		for _, dependency := range pkg.Imports {
			visit(dependency)
		}
	}
	visit(root)

	return seen
}

func withinPackage(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}
