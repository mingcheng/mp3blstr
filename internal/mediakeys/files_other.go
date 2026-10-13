//go:build !linux

package mediakeys

func ForwardFiles([]string, bool) (bool, error) { return false, nil }
