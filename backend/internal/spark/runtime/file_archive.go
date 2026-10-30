package runtime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"

	"campfire/internal/shared/model"
)

func (m *Manager) archiveFiles(ctx context.Context, d model.Deployment, root *os.Root, volume string, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Destination}

	sources := []string{q.Path}
	if len(q.Paths) > 0 {
		if q.Operation != "compress" || len(q.Paths) > 1000 {
			return result, fileFailure("invalid_file", "Select at most 1,000 archive items.")
		}

		sources = q.Paths

		seen := make(map[string]bool, len(sources))
		for _, source := range sources {
			if !filePath(source, false) || path.Dir(source) != q.Path || seen[source] {
				return result, fileFailure("invalid_file", "Select distinct items from the current directory.")
			}

			seen[source] = true
		}
	}

	for _, source := range sources {
		if source == q.Destination || strings.HasPrefix(q.Destination, source+"/") {
			return result, fileFailure("invalid_file", "Choose a destination outside the selected items.")
		}

		if err := containedPath(root, source, false); err != nil {
			return result, err
		}
	}

	if !filePath(q.Destination, false) {
		return result, fileFailure("invalid_file", "Choose a new destination outside the source directory.")
	}

	if err := containedPath(root, q.Path, false); err != nil {
		return result, err
	}

	if err := containedPath(root, q.Destination, true); err != nil {
		return result, err
	}

	if _, err := root.Lstat(q.Destination); !errors.Is(err, os.ErrNotExist) {
		return result, fileFailure("file_conflict", "Archive destination already exists.")
	}

	temporary := ".campfire-archive-" + rand.Text()
	defer func() { _ = root.RemoveAll(temporary) }()

	var err error

	if q.Operation == "compress" {
		if !strings.HasSuffix(q.Destination, ".tar.gz") {
			return result, fileFailure("invalid_file", "Compressed archives must end in .tar.gz.")
		}

		err = m.compressFiles(ctx, d, root, sources, temporary)
	} else {
		err = root.Mkdir(temporary, 0700)
		if err == nil {
			err = m.extractFiles(ctx, d, root, q.Path, temporary)
		}
	}

	if err != nil {
		var failure *model.FileError
		if errors.As(err, &failure) {
			return result, err
		}

		return result, fileFailure("file_io", "Archive operation failed. Check the archive, disk allocation and Spark logs.")
	}

	if err := checkWriteCapacity(ctx, volume, d.Configuration.DiskMB, 0, 0); err != nil {
		return result, err
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	if err := root.Rename(temporary, q.Destination); err != nil {
		return result, fileFailure("file_io", "Could not publish archive result.")
	}

	return result, nil
}

func (m *Manager) compressFiles(ctx context.Context, d model.Deployment, root *os.Root, sources []string, temporary string) error {
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	output := &transferWriter{Writer: file}
	zipped := gzip.NewWriter(output)
	archive := tar.NewWriter(zipped)

	var total int64

	count := 0

	for _, source := range sources {
		err = fs.WalkDir(root.FS(), source, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if reservedFilePath(name) {
				if entry.IsDir() {
					return fs.SkipDir
				}

				return nil
			}

			if name == "." {
				return nil
			}

			count++
			if count > backupEntries || !filePath(name, false) {
				return fileFailure("file_limit", "Archives are limited to 100,000 entries.")
			}

			if err := containedPath(root, name, false); err != nil {
				return err
			}

			input, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}

			defer func() { _ = input.Close() }()

			info, err := input.Stat()
			if err != nil {
				return err
			}

			if !info.IsDir() && !info.Mode().IsRegular() {
				return fileFailure("invalid_file", "Archives cannot contain links or special files.")
			}

			total += info.Size()
			if total > model.FileTransferLimit {
				return fileFailure("file_limit", "Archive contents exceed 100 GiB.")
			}

			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}

			header.Name = name
			if source != "." {
				header.Name = strings.TrimPrefix(name, path.Dir(source)+"/")
			}

			header.Mode = header.Mode & 0777

			header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
			if err := archive.WriteHeader(header); err != nil {
				return err
			}

			if info.Mode().IsRegular() {
				_, err = io.CopyN(archive, backupReader{ctx, input}, info.Size())
			}

			return err
		})
		if err != nil {
			break
		}
	}

	closeArchive, closeZip := archive.Close(), zipped.Close()

	if err != nil {
		return err
	}

	if closeArchive != nil {
		return closeArchive
	}

	if closeZip != nil {
		return closeZip
	}

	if err := m.FileOwner(file, d.Ember.User); err != nil {
		return err
	}

	return file.Sync()
}

func (m *Manager) extractFiles(ctx context.Context, d model.Deployment, root *os.Root, source, stage string) error {
	input, err := root.OpenFile(source, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()

	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fileFailure("invalid_file", "Select a regular archive file.")
	}

	if info.Size() > model.FileTransferLimit {
		return fileFailure("file_limit", "Archive exceeds 100 GiB.")
	}

	used, err := fileUsage(ctx, root.Name())
	if err != nil {
		return err
	}

	budget := model.FileTransferLimit
	if d.Configuration.DiskMB > 0 {
		budget = max(int64(0), d.Configuration.DiskMB*1048576-used)
	}

	target, err := root.OpenRoot(stage)
	if err != nil {
		return err
	}

	defer func() { _ = target.Close() }()

	var total int64

	count := 0
	seen := make(map[string]bool)
	directoryModes := make(map[string]fs.FileMode)
	unpack := func(name string, size int64, mode fs.FileMode, reader io.Reader) error {
		count++
		if count > backupEntries || size < 0 || size > model.FileTransferLimit-total {
			return fileFailure("file_limit", "Archive expands beyond 100 GiB or 100,000 entries.")
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if mode.IsDir() && size != 0 {
			return fileFailure("invalid_file", "Archive directories cannot carry file contents.")
		}

		name = strings.TrimSuffix(name, "/")
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}

		if (name == "." || name == "") && mode.IsDir() && size == 0 {
			return nil
		}

		if !filePath(name, false) || seen[name] || !mode.IsDir() && !mode.IsRegular() {
			return fileFailure("invalid_file", "Archive contains unsafe, duplicate or unsupported entries.")
		}

		seen[name] = true

		total += size

		if mode.IsDir() {
			directoryModes[name] = importedMode(mode)
			return target.MkdirAll(name, 0700)
		}

		if total > budget {
			return fileFailure("file_limit", "Extracted files would exceed the server disk allocation.")
		}

		if err := target.MkdirAll(path.Dir(name), 0700); err != nil {
			return err
		}

		file, err := target.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}

		defer func() { _ = file.Close() }()

		if _, err := io.CopyN(file, backupReader{ctx, reader}, size); err != nil {
			return err
		}

		if err := file.Chmod(importedMode(mode)); err != nil {
			return err
		}

		return file.Sync()
	}

	if strings.HasSuffix(strings.ToLower(source), ".zip") {
		if err := checkZIPDirectory(input, info.Size()); err != nil {
			return err
		}

		archive, err := zip.NewReader(input, info.Size())
		if err != nil {
			return err
		}

		if len(archive.File) > backupEntries {
			return fileFailure("file_limit", "Archive has too many entries.")
		}

		for _, item := range archive.File {
			if item.UncompressedSize64 > uint64(model.FileTransferLimit) {
				return fileFailure("file_limit", "Archive expands beyond 100 GiB.")
			}

			reader, err := item.Open()
			if err != nil {
				return err
			}

			err = unpack(item.Name, int64(item.UncompressedSize64), item.Mode(), reader)
			// Read through the checksum before accepting a ZIP entry.
			if err == nil {
				var tail [1]byte

				n, end := reader.Read(tail[:])
				if n != 0 || end != io.EOF {
					err = errors.New("invalid ZIP entry checksum or length")
				}
			}

			_ = reader.Close()

			if err != nil {
				return err
			}
		}
	} else if strings.HasSuffix(strings.ToLower(source), ".rar") {
		if err := extractRAR(backupReader{ctx, input}, unpack); err != nil {
			return err
		}
	} else if strings.HasSuffix(strings.ToLower(source), ".tar.gz") || strings.HasSuffix(strings.ToLower(source), ".tgz") {
		compressed, err := gzip.NewReader(input)
		if err != nil {
			return err
		}
		defer func() { _ = compressed.Close() }()

		archive := tar.NewReader(compressed)
		for {
			header, err := archive.Next()
			if err == io.EOF {
				break
			}

			if err != nil {
				return err
			}

			var mode fs.FileMode

			switch header.Typeflag {
			case tar.TypeDir:
				mode = fs.ModeDir | fs.FileMode(header.Mode)&0777
			case tar.TypeReg, 0:
				mode = fs.FileMode(header.Mode) & 0777
			default:
				return fileFailure("invalid_file", "Archives cannot contain links or special files.")
			}

			if err := unpack(header.Name, header.Size, mode, archive); err != nil {
				return err
			}
		}
		// Validate gzip's trailer without accepting unbounded trailing data.
		n, err := io.Copy(io.Discard, io.LimitReader(backupReader{ctx, compressed}, model.FileLimit+1))
		if err != nil {
			return err
		}

		if n > model.FileLimit {
			return fileFailure("invalid_file", "Unexpected trailing archive data.")
		}
	} else {
		return fileFailure("invalid_file", "Extract supports .zip, .tar.gz, .tgz and .rar archives.")
	}
	// Assign all implicitly created directories as well as files to the game's user.
	err = fs.WalkDir(target.FS(), ".", func(name string, _ fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		file, err := target.Open(name)
		if err != nil {
			return err
		}

		defer func() { _ = file.Close() }()

		return m.FileOwner(file, d.Ember.User)
	})
	if err != nil {
		return err
	}

	directories := make([]string, 0, len(directoryModes))
	for name := range directoryModes {
		directories = append(directories, name)
	}

	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })

	for _, name := range directories {
		if err := target.Chmod(name, directoryModes[name]); err != nil {
			return err
		}
	}

	return nil
}
