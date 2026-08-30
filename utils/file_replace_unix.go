//go:build !windows

package utils

import "os"

func replaceFile(source, target string) error {
	return os.Rename(source, target)
}
