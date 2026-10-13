//go:build !unix

package main

import "errors"

// chroot is not available on this platform.
func chroot(string) error { return errors.New("chroot is not supported on this platform") }
