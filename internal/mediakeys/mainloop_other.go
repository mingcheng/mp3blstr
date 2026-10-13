//go:build !darwin

package mediakeys

import "os"

// Main runs fn and exits with its result. Only macOS needs the main thread
// for media keys; elsewhere fn simply runs on the calling goroutine.
func Main(fn func() int) { os.Exit(fn()) }
