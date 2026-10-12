//go:build unix

package main

import "syscall"

func chroot(dir string) error {
	if err := syscall.Chroot(dir); err != nil {
		return err
	}
	return syscall.Chdir("/")
}
