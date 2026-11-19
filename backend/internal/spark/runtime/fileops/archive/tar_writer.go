package archive

import "archive/tar"

// Server-file callers use neutral metadata; portable backups retain their
// explicit tar-header API and fixed format through TarGzipWriter.
type tarFileWriter struct{ *TarGzipWriter }

func (w *tarFileWriter) WriteHeader(entry Entry) error {
	if err := validateWriteEntry(entry); err != nil {
		return err
	}

	header := &tar.Header{Name: entry.Name, Size: entry.Size, Mode: int64(entry.Mode.Perm()), ModTime: entry.Modified, Typeflag: tar.TypeReg}
	if entry.Mode.IsDir() {
		header.Typeflag = tar.TypeDir
	}

	return w.TarGzipWriter.WriteHeader(header)
}
