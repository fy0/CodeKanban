//go:build windows

package websession

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPiCommandUsesCmdForWindowsShim(t *testing.T) {
	cmd, err := buildPiCommand(context.Background(), `C:\tools\pi.cmd`, "--version")
	if err != nil {
		t.Fatalf("buildPiCommand returned error: %v", err)
	}
	if !strings.EqualFold(filepath.Base(cmd.Path), "cmd.exe") {
		t.Fatalf("command path = %q, want cmd.exe", cmd.Path)
	}
	commandLine := cmd.SysProcAttr.CmdLine
	if !strings.Contains(commandLine, `C:\tools\pi.cmd`) || !strings.Contains(commandLine, "--version") {
		t.Fatalf("unexpected shim command line: %q", commandLine)
	}
}

func TestProbePiRuntimeExecutesWindowsCmdShimWithSpaces(t *testing.T) {
	t.Setenv("CODEKANBAN_PI_PROBE_HELPER", "1")
	t.Setenv("CODEKANBAN_PI_PROBE_VERSION", "0.84.1")
	dir := filepath.Join(t.TempDir(), "Pi Command")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create shim dir: %v", err)
	}
	shimPath := filepath.Join(dir, "pi.cmd")
	content := fmt.Sprintf(
		"@echo off\r\n\"%s\" -test.run=TestPiProbeHelperProcess -- %%*\r\n",
		os.Args[0],
	)
	if err := os.WriteFile(shimPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	result := probePiRuntime(`"`+shimPath+`"`, t.TempDir())
	if !result.installed || !result.compatible || result.version == nil || *result.version != "0.84.1" {
		t.Fatalf("unexpected cmd shim probe result: %#v", result)
	}
}
