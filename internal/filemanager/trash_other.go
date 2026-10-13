//go:build !darwin

package filemanager

import "errors"

func Trash(string) error {
	return errors.New("files.use_trash is currently supported only on macOS; no file was deleted")
}
