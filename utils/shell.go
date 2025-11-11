package utils

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/shlex"
)

// ResolveShellCommand selects an available shell command for the current host.
// override takes precedence. When override is empty, the configured shell plus
// platform defaults are probed in order until a binary is found.
func ResolveShellCommand(override string, cfg TerminalShellConfig) ([]string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		return parsePreferredShell(override)
	}

	candidates := buildShellCandidates(cfg)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no shell candidates configured for %s", runtime.GOOS)
	}

	var attempted []string
	for _, candidate := range candidates {
		parts, err := shlex.Split(candidate)
		if err != nil {
			return nil, fmt.Errorf("invalid shell specification %q: %w", candidate, err)
		}
		if len(parts) == 0 {
			continue
		}
		if err := ensureExecutable(parts[0]); err != nil {
			attempted = append(attempted, parts[0])
			continue
		}
		return parts, nil
	}

	if len(attempted) > 0 {
		return nil, fmt.Errorf("no suitable shell found for %s (tried %s)", runtime.GOOS, strings.Join(attempted, ", "))
	}
	return nil, fmt.Errorf("no suitable shell found for %s", runtime.GOOS)
}

func parsePreferredShell(raw string) ([]string, error) {
	parts, err := shlex.Split(raw)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid shell configuration: %q", raw)
	}
	if err := ensureExecutable(parts[0]); err != nil {
		return nil, fmt.Errorf("shell %q not found: %w", parts[0], err)
	}
	return parts, nil
}

func buildShellCandidates(cfg TerminalShellConfig) []string {
	var candidates []string
	appendCandidate := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		for _, existing := range candidates {
			if strings.EqualFold(existing, raw) {
				return
			}
		}
		candidates = append(candidates, raw)
	}

	switch runtime.GOOS {
	case "windows":
		appendCandidate(cfg.Windows)
		appendCandidate("pwsh.exe -NoLogo")
		appendCandidate("powershell.exe -NoLogo")
		appendCandidate("cmd.exe")
	case "darwin":
		appendCandidate(cfg.Darwin)
		appendCandidate("/bin/zsh")
		appendCandidate("/bin/bash")
		appendCandidate("/bin/sh")
	default:
		appendCandidate(cfg.Linux)
		appendCandidate("/bin/bash")
		appendCandidate("/bin/sh")
	}

	return candidates
}

func ensureExecutable(name string) error {
	_, err := exec.LookPath(name)
	return err
}
