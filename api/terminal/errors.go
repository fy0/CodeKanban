package terminal

import "errors"

var (
	// ErrSessionNotFound indicates the referenced session cannot be located.
	ErrSessionNotFound = errors.New("terminal session not found")
	// ErrSessionLimitReached indicates the project exceeded the allowed number of sessions.
	ErrSessionLimitReached = errors.New("terminal session limit reached")
)
