package executorclient

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrUnavailable = errors.New("unavailable")
	ErrBadRequest  = errors.New("invalid argument")
	ErrForbidden   = errors.New("permission denied")
)

// mapError converts a ConnectRPC error into a domain error.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch connect.CodeOf(err) {
	case connect.CodeNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case connect.CodeInvalidArgument:
		return fmt.Errorf("%w: %s", ErrBadRequest, msg)
	case connect.CodePermissionDenied:
		return fmt.Errorf("%w: %s", ErrForbidden, msg)
	case connect.CodeUnavailable, connect.CodeAborted:
		return fmt.Errorf("%w: %s", ErrUnavailable, msg)
	default:
		return err
	}
}
