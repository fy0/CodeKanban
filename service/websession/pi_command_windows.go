//go:build windows

package websession

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
)

func buildWindowsBatchCommand(
	ctx context.Context,
	comspec string,
	batchPath string,
	args []string,
) *exec.Cmd {
	quotedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		quotedArgs = append(quotedArgs, quoteWindowsBatchArgument(arg))
	}
	commandLine := `/d /s /c ""` + strings.ReplaceAll(batchPath, `"`, `""`) + `"`
	if len(quotedArgs) > 0 {
		commandLine += " " + strings.Join(quotedArgs, " ")
	}
	commandLine += `"`

	cmd := exec.CommandContext(ctx, comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine}
	return cmd
}

func quoteWindowsBatchArgument(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
