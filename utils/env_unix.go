//go:build !windows

package utils

import "os"

// GetFreshEnviron returns the current environment variables.
// On Unix systems, there's no central registry for environment variables,
// so this just returns os.Environ().
// If you need to pick up changes from shell profiles (.bashrc, .zshrc),
// the shell itself will handle that when started as a login shell.
func GetFreshEnviron() []string {
	return os.Environ()
}

// GetFreshPath returns the current PATH environment variable.
// On Unix, environment changes require re-sourcing shell profiles.
func GetFreshPath() string {
	return os.Getenv("PATH")
}
