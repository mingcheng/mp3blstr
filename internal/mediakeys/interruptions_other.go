//go:build !darwin

package mediakeys

import (
	"errors"
	"io"
)

const SupportsInterruptions = false

func WatchInterruptions(bool, bool, func(Interruption)) (io.Closer, error) {
	return nil, errors.New("system interruption monitoring is only supported on macOS")
}
