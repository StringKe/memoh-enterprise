package security

import (
	"errors"
	"time"
)

const (
	DefaultExecTimeout = 30 * time.Second
	MaxExecTimeout     = 600 * time.Second
	PTYIdleTimeout     = 300 * time.Second
)

var ErrTimeoutTooLarge = errors.New("timeout exceeds maximum")

func NormalizeExecTimeout(seconds int32) (time.Duration, error) {
	if seconds <= 0 {
		return DefaultExecTimeout, nil
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > MaxExecTimeout {
		return 0, ErrTimeoutTooLarge
	}
	return timeout, nil
}
