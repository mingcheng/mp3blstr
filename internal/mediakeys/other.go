//go:build !linux && !windows && !darwin

package mediakeys

import "errors"

func Start(func(Command), func(error)) (Controller, error) {
	return nil, errors.New("global media keys are not available on this platform")
}
