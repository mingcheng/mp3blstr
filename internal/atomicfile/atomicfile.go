// Package atomicfile replaces files atomically: readers (and a crash halfway
// through) see either the old or the new contents, never a truncated file.
package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Write replaces the file at path with data. The data goes to a temporary
// file in the same directory, which is then renamed over path.
//
// Symbolic links are followed, so the target is replaced and the link is
// kept. The permission bits of an existing file are preserved; perm applies
// to new files only.
func Write(path string, data []byte, perm fs.FileMode) error {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	// Cleans up after a failure; after the rename the name no longer exists.
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
