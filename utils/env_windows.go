//go:build windows

package utils

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func expandWindowsEnvironmentStrings(value string) (string, error) {
	src, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return "", err
	}

	// First call with size=0 to get required buffer length (in UTF-16 code units, including NUL).
	n, err := windows.ExpandEnvironmentStrings(src, nil, 0)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}

	buf := make([]uint16, n)
	n2, err := windows.ExpandEnvironmentStrings(src, &buf[0], n)
	if err != nil {
		return "", err
	}
	if n2 == 0 {
		return "", nil
	}

	// buf includes trailing NUL
	return windows.UTF16ToString(buf), nil
}

// GetFreshEnviron returns the current environment variables with PATH refreshed from Windows registry.
// This is useful for terminal managers that need to pick up newly installed tools.
func GetFreshEnviron() []string {
	// Start with current environment
	env := os.Environ()

	// Get fresh PATH from registry
	freshPath := GetFreshPath()
	if freshPath == "" {
		return env
	}

	// Replace PATH in the environment
	result := make([]string, 0, len(env))
	pathReplaced := false
	for _, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			result = append(result, "PATH="+freshPath)
			pathReplaced = true
		} else {
			result = append(result, e)
		}
	}

	if !pathReplaced {
		result = append(result, "PATH="+freshPath)
	}

	return result
}

// GetFreshPath reads and combines PATH from Windows registry (system + user).
// Returns the combined PATH or empty string on error.
func GetFreshPath() string {
	var paths []string

	// System PATH (HKEY_LOCAL_MACHINE)
	if sysPath := getRegistryPath(
		registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		"Path",
	); sysPath != "" {
		paths = append(paths, sysPath)
	}

	// User PATH (HKEY_CURRENT_USER)
	if userPath := getRegistryPath(
		registry.CURRENT_USER,
		`Environment`,
		"Path",
	); userPath != "" {
		paths = append(paths, userPath)
	}

	if len(paths) == 0 {
		return ""
	}

	return strings.Join(paths, ";")
}

// getRegistryPath reads a string value from the Windows registry.
func getRegistryPath(root registry.Key, keyPath, valueName string) string {
	key, err := registry.OpenKey(root, keyPath, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	// Try to read as expandable string first (REG_EXPAND_SZ)
	value, valType, err := key.GetStringValue(valueName)
	if err != nil {
		return ""
	}

	// Expand environment variables.
	// Registry values commonly use Windows-style %VAR% expansions (e.g. %SystemRoot%).
	// Some environments store PATH as REG_SZ even when it contains %VAR%, so expand regardless of type.
	if expanded, err := expandWindowsEnvironmentStrings(value); err == nil && expanded != "" {
		value = expanded
	} else if valType == registry.EXPAND_SZ {
		// Best-effort fallback when ExpandEnvironmentStrings fails for some reason.
		// Note: os.ExpandEnv doesn't handle %VAR% on Windows; this is only useful for $VAR style.
		value = os.ExpandEnv(value)
	}

	return value
}
