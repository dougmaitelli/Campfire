package runtime

import (
	"errors"
	"os"
	"path"
	"strings"
)

func configParents(root *os.Root, name string, create bool) error {
	if !filePath(name, false) {
		return errors.New("invalid configuration path")
	}

	parts := strings.Split(name, "/")
	for index := range parts {
		info, err := root.Lstat(strings.Join(parts[:index+1], "/"))
		if errors.Is(err, os.ErrNotExist) && create {
			break
		}

		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("configuration links and special files are unsupported")
		}

		if index < len(parts)-1 && !info.IsDir() {
			return errors.New("configuration parent is not a directory")
		}
	}

	if create {
		return root.MkdirAll(path.Dir(name), 0700)
	}

	return nil
}
