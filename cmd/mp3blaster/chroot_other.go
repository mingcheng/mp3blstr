//go:build !unix

package main

import "errors"

func chroot(string) error { return errors.New("chroot is not supported on this platform") }
