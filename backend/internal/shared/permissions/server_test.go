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
	for _, required := range []string{ServerView, ServerOperate, ServerConsoleRead, ServerBackupsRead} {
		if !slices.Contains(restore, required) {
			t.Fatal("restore dependency missing", required, restore)
		}
	}

	if slices.Contains(restore, ServerFilesWrite) || slices.Contains(restore, ServerFilesRead) {
		t.Fatal("restore granted file browser access", restore)
	}

	if slices.Contains(Expand([]string{ServerOperate, ServerFilesWrite}), ServerBackupsRestore) {
		t.Fatal("prerequisites granted restore")
	}

	withoutOperate := slices.DeleteFunc(slices.Clone(restore), func(id string) bool { return id == ServerOperate })

	filtered := Restrict(withoutOperate)
	if slices.Contains(filtered, ServerBackupsRestore) || slices.Contains(filtered, ServerOperate) {
		t.Fatal("token restriction was undone", filtered)
	}
}

func TestCatalogIsOnePermissionTree(t *testing.T) {
	seen := map[string]bool{}

	for _, item := range Catalog() {
		if item.ID == ServerView && len(item.Requires) != 0 || item.ID != ServerView && len(item.Requires) != 1 {
			t.Fatal("every permission must have exactly one parent except server.view", item)
		}

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

func TestOperationParentsAndScopeRevocation(t *testing.T) {
	for _, action := range []string{ServerFilesRead, ServerBackupsRead, ServerSchedulesRead, ServerConfigure, ServerReinstall, ServerDelete} {
		expanded := Expand([]string{action})
		for _, parent := range []string{ServerOperate, ServerConsoleRead, ServerView} {
			if !slices.Contains(expanded, parent) {
				t.Fatal("operation ancestor missing", action, parent, expanded)
			}
		}

		limited := Restrict(slices.DeleteFunc(slices.Clone(expanded), func(id string) bool { return id == ServerOperate }))
		if slices.Contains(limited, action) {
			t.Fatal("permission survived removal of operation scope", action, limited)
		}
	}

	reinstall := Expand([]string{ServerReinstall})
	for _, unrelated := range []string{ServerConfigure, ServerFilesRead, ServerFilesWrite, ServerFilesDelete, ServerBackupsRead, ServerSchedulesRead} {
		if slices.Contains(reinstall, unrelated) {
			t.Fatal("reinstall granted another branch", unrelated, reinstall)
		}
	}

	limited := Restrict(slices.DeleteFunc(Expand(All()), func(id string) bool { return id == ServerOperate }))
	if !slices.Equal(limited, []string{ServerConsoleRead, ServerView}) {
		t.Fatal("removing operation scope did not prune its complete subtree", limited)
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

	if !slices.Equal(Legacy([]string{ServerView}), []string{ServerView}) {
		t.Fatal("legacy overview access acquired operation through schedules")
	}
}
