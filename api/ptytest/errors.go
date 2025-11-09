package ptytest

import "errors"

var (
	// ErrSessionNotFound indicates the requested PTY test session does not exist.
	ErrSessionNotFound = errors.New("pty test session not found")
	// ErrInvalidEncoding indicates the provided encoding is not supported.
	ErrInvalidEncoding = errors.New("unsupported encoding")
)
