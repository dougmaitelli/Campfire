package model

import "testing"

func TestArchiveFormatNames(t *testing.T) {
	for _, item := range []struct {
		name string
		want ArchiveFormat
	}{
		{"save.zip", ArchiveZIP}, {"save.rar", ArchiveRAR}, {"save.TAR", ArchiveTar},
		{"save.tar.gz", ArchiveTarGzip}, {"save.tgz", ArchiveTarGzip},
		{"save.tar.xz", ArchiveTarXZ}, {"save.TXZ", ArchiveTarXZ},
		{"save.tar.lz", ArchiveTarLzip}, {"save.tlz", ArchiveTarLzip},
		{"save.tar.bz2", ArchiveTarBzip2}, {"save.tbz2", ArchiveTarBzip2}, {"save.tbz", ArchiveTarBzip2},
		{"save.tar.lz4", ArchiveTarLZ4},
		{"save.tar.zstd", ArchiveTarZstd}, {"save.tar.zst", ArchiveTarZstd}, {"save.tzst", ArchiveTarZstd},
		{"save.7z", ""}, {"save.xz", ""}, {"save.zip.001", ""}, {"save.tar.gz.exe", ""},
	} {
		if got := ArchiveFormatForName(item.name); got != item.want {
			t.Fatal(item.name, got, item.want)
		}
	}
}
