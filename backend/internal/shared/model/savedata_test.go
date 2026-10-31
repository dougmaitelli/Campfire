package model

import (
	"strings"
	"testing"
)

func TestBackupModeArchiveNames(t *testing.T) {
	for _, mode := range []BackupMode{BackupModeFull, BackupModeSavedata} {
		name := mode.ArchiveName(strings.Repeat("a", 128))
		if !strings.HasPrefix(name, string(mode)+"-") || !BackupName(name) {
			t.Fatal("mode prefix does not support a full operation identifier", name)
		}
	}
}

func TestSavedataPathsValidation(t *testing.T) {
	for _, name := range []string{"", ".", "/world", "../world", "world/../config", "world//save", "world/", "world\\save", "world/*", "world/$SAVE", BackupMetadataName, "world\n"} {
		if err := ValidateSavedataPaths([]SavedataPath{{Path: name}}); err == nil {
			t.Errorf("accepted unsafe or ambiguous path %q", name)
		}
	}

	if err := ValidateSavedataPaths([]SavedataPath{{Path: "world"}, {Path: "world/players"}}); err == nil {
		t.Fatal("accepted overlapping roots")
	}

	if err := ValidateSavedataPaths([]SavedataPath{{Path: "world", Required: true}, {Path: "server.properties"}}); err != nil {
		t.Fatal(err)
	}

	if err := (BackupOptions{Mode: BackupModeSavedata}).Validate(); err == nil {
		t.Fatal("accepted save-data backup without declared paths")
	}

	if err := (BackupOptions{Mode: "unknown"}).Validate(); err == nil {
		t.Fatal("accepted unknown mode")
	}
}
