//go:build !linux && !windows && !darwin

package mediakeys

import "errors"

func Start(func(Action)) (Controller, error) {
	return nil, errors.New("global media keys are not available on this platform")
}
