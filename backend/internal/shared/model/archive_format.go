package model

import "strings"

// ArchiveFormat identifies server-file formats, not portable backup formats.
type ArchiveFormat string

// Supported server-file archive formats.
const (
	ArchiveZIP      ArchiveFormat = "zip"
	ArchiveRAR      ArchiveFormat = "rar"
	ArchiveTar      ArchiveFormat = "tar"
	ArchiveTarGzip  ArchiveFormat = "tar.gz"
	ArchiveTarXZ    ArchiveFormat = "tar.xz"
	ArchiveTarLzip  ArchiveFormat = "tar.lz"
	ArchiveTarBzip2 ArchiveFormat = "tar.bz2"
	ArchiveTarLZ4   ArchiveFormat = "tar.lz4"
	ArchiveTarZstd  ArchiveFormat = "tar.zst"
)

// ArchiveFormatForName recognizes complete, case-insensitive archive suffixes.
func ArchiveFormatForName(name string) ArchiveFormat {
	name = strings.ToLower(name)

	for _, item := range []struct {
		format   ArchiveFormat
		suffixes []string
	}{
		{ArchiveZIP, []string{".zip"}},
		{ArchiveRAR, []string{".rar"}},
		{ArchiveTarGzip, []string{".tar.gz", ".tgz"}},
		{ArchiveTarXZ, []string{".tar.xz", ".txz"}},
		{ArchiveTarLzip, []string{".tar.lz", ".tlz"}},
		{ArchiveTarBzip2, []string{".tar.bz2", ".tbz2", ".tbz"}},
		{ArchiveTarLZ4, []string{".tar.lz4"}},
		{ArchiveTarZstd, []string{".tar.zst", ".tar.zstd", ".tzst"}},
		{ArchiveTar, []string{".tar"}},
	} {
		for _, suffix := range item.suffixes {
			if strings.HasSuffix(name, suffix) {
				return item.format
			}
		}
	}

	return ""
}

// IsTar identifies formats sharing tar member and conflict handling.
func (f ArchiveFormat) IsTar() bool {
	switch f {
	case ArchiveTar, ArchiveTarGzip, ArchiveTarXZ, ArchiveTarLzip, ArchiveTarBzip2, ArchiveTarLZ4, ArchiveTarZstd:
		return true
	default:
		return false
	}
}
