package permissions

import (
	"slices"
	"testing"
)

func TestFeatureDependenciesAndTokenIntersection(t *testing.T) {
	power := Expand([]string{ServerOperate})
	if !slices.Contains(power, ServerConsoleRead) || slices.Contains(power, ServerFilesRead) || slices.Contains(power, ServerConsoleWrite) {
		t.Fatal("operation access included unrelated features", power)
	}

	restore := Expand([]string{ServerBackupsRestore})
	for _, required := range []string{ServerView, ServerFilesRead, ServerFilesWrite, ServerOperate, ServerConsoleRead, ServerBackupsRead} {
		if !slices.Contains(restore, required) {
			t.Fatal("restore dependency missing", required, restore)
		}
	}

	if slices.Contains(Expand([]string{ServerOperate, ServerFilesWrite}), ServerBackupsRestore) {
		t.Fatal("prerequisites granted restore")
	}

	withoutWrite := slices.DeleteFunc(slices.Clone(restore), func(id string) bool { return id == ServerFilesWrite })

	filtered := Restrict(withoutWrite)
	if slices.Contains(filtered, ServerBackupsRestore) || slices.Contains(filtered, ServerFilesWrite) {
		t.Fatal("token restriction was undone", filtered)
	}
}

func TestCatalogHasUniqueIdentifiersAndAcyclicDependencies(t *testing.T) {
	seen := map[string]bool{}
	for _, item := range Catalog() {
		if seen[item.ID] {
			t.Fatal("duplicate permission", item.ID)
		}

		seen[item.ID] = true

		var visit func(string, map[string]bool)

		visit = func(id string, active map[string]bool) {
			if active[id] {
				t.Fatal("permission dependency cycle", id)
			}

			if !Valid(id) {
				t.Fatal("unknown dependency", id)
			}

			active[id] = true
			for _, definition := range server {
				if definition.ID == id {
					for _, dependency := range definition.Requires {
						visit(dependency, active)
					}
				}
			}

			delete(active, id)
		}
		visit(item.ID, map[string]bool{})
	}
}

func TestLegacyGrantsAreExpandedOnce(t *testing.T) {
	legacy := Legacy([]string{ServerView, ServerOperate, ServerConfigure, LegacyServerShare})
	for _, id := range []string{ServerConsoleWrite, ServerFilesDelete, ServerBackupsCreate, ServerBackupsDownload, ServerBackupsRestore, ServerBackupsDelete, ServerReinstall, ServerSchedulesManage, ServerSchedulesRun} {
		if !slices.Contains(legacy, id) {
			t.Fatal("legacy access lost", id)
		}
	}

	if slices.Contains(legacy, LegacyServerShare) {
		t.Fatal("sharing permission survived migration")
	}

	if slices.Contains(Expand([]string{ServerOperate}), ServerFilesRead) {
		t.Fatal("legacy expansion is still implicit")
	}
}
