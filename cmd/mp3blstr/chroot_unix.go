//go:build unix

package main

import "syscall"

// chroot changes the root directory to dir (--chroot); it needs root
// privileges.
func chroot(dir string) error {
	if err := syscall.Chroot(dir); err != nil {
		return err
	}
	return syscall.Chdir("/")
}
